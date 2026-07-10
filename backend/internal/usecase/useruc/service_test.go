package useruc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

type fakeUsers struct {
	user          *auth.User
	updatedName   string
	updatedAvatar string
	deleted       bool
}

func (f *fakeUsers) GetByID(_ context.Context, _ string) (*auth.User, error) {
	if f.user == nil {
		return nil, domain.ErrNotFound
	}
	return f.user, nil
}
func (f *fakeUsers) UpdateName(_ context.Context, _, name string) error {
	f.updatedName = name
	return nil
}
func (f *fakeUsers) UpdateAvatarKey(_ context.Context, _, key string) error {
	f.updatedAvatar = key
	return nil
}
func (f *fakeUsers) Delete(_ context.Context, _ string) error { f.deleted = true; return nil }

type fakeAvatars struct{}

func (fakeAvatars) PresignPut(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "https://minio/put", nil
}
func (fakeAvatars) PresignGet(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "https://minio/get", nil
}

type fakeSessions struct{ revoked bool }

func (f *fakeSessions) RevokeAllForUser(_ context.Context, _, _ string) error {
	f.revoked = true
	return nil
}

type fakeSoleOwner struct {
	orgs []tenant.Organization
	err  error
}

func (f *fakeSoleOwner) SoleOwnerOrgs(_ context.Context, _ string) ([]tenant.Organization, error) {
	return f.orgs, f.err
}

func TestUpdateName_Validation(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{}
	s := New(Deps{Users: users})

	if err := s.UpdateName(ctx, "u", "   "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("blank name err = %v, want ErrValidation", err)
	}
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	if err := s.UpdateName(ctx, "u", string(long)); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("101-char name err = %v, want ErrValidation", err)
	}
	if err := s.UpdateName(ctx, "u", "  Alice  "); err != nil {
		t.Fatalf("valid name: %v", err)
	}
	if users.updatedName != "Alice" {
		t.Fatalf("stored name = %q, want trimmed Alice", users.updatedName)
	}
}

func TestConfirmAvatar_KeyOwnership(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{}
	s := New(Deps{Users: users})

	// A key outside the caller's namespace is rejected (no confirming someone else's).
	if err := s.ConfirmAvatar(ctx, "u1", "avatars/u2/abc"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("foreign key err = %v, want ErrValidation", err)
	}
	if err := s.ConfirmAvatar(ctx, "u1", "avatars/u1/abc"); err != nil {
		t.Fatalf("own key: %v", err)
	}
	if users.updatedAvatar != "avatars/u1/abc" {
		t.Fatalf("stored key = %q", users.updatedAvatar)
	}
}

func TestRequestAvatarUpload_Validation(t *testing.T) {
	ctx := context.Background()
	s := New(Deps{Users: &fakeUsers{}, Avatars: fakeAvatars{}})

	if _, _, err := s.RequestAvatarUpload(ctx, "u", "application/pdf", 10); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("bad type err = %v, want ErrValidation", err)
	}
	if _, _, err := s.RequestAvatarUpload(ctx, "u", "image/png", 0); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("zero size err = %v, want ErrValidation", err)
	}
	if _, _, err := s.RequestAvatarUpload(ctx, "u", "image/png", 6<<20); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("oversize err = %v, want ErrValidation", err)
	}
	key, url, err := s.RequestAvatarUpload(ctx, "u", "image/png", 1024)
	if err != nil {
		t.Fatalf("valid upload: %v", err)
	}
	if url == "" || key == "" {
		t.Fatalf("empty key/url: key=%q url=%q", key, url)
	}
}

func TestRequestAvatarUpload_Disabled(t *testing.T) {
	// No AvatarStore wired ⇒ uploads forbidden.
	s := New(Deps{Users: &fakeUsers{}})
	if _, _, err := s.RequestAvatarUpload(context.Background(), "u", "image/png", 1024); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestDelete_SoleOwnerBlocks(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{}
	sessions := &fakeSessions{}
	owner := &fakeSoleOwner{orgs: []tenant.Organization{{ID: "o1", Slug: "acme", Name: "Acme"}}}
	s := New(Deps{Users: users, Sessions: sessions, SoleOwner: owner})

	var soleErr *SoleOwnerError
	err := s.Delete(ctx, "u")
	if !errors.As(err, &soleErr) {
		t.Fatalf("err = %v, want *SoleOwnerError", err)
	}
	if len(soleErr.Orgs) != 1 || soleErr.Orgs[0].Slug != "acme" {
		t.Fatalf("blocked orgs = %+v", soleErr.Orgs)
	}
	if users.deleted || sessions.revoked {
		t.Fatalf("must not delete/revoke when sole owner")
	}
}

func TestDelete_HappyPath(t *testing.T) {
	ctx := context.Background()
	users := &fakeUsers{}
	sessions := &fakeSessions{}
	s := New(Deps{Users: users, Sessions: sessions, SoleOwner: &fakeSoleOwner{}})

	if err := s.Delete(ctx, "u"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !sessions.revoked {
		t.Fatalf("sessions not revoked before delete")
	}
	if !users.deleted {
		t.Fatalf("user not deleted")
	}
}

func TestDelete_Disabled(t *testing.T) {
	// No SoleOwnerReader (owner pool absent) ⇒ deletion disabled.
	s := New(Deps{Users: &fakeUsers{}, Sessions: &fakeSessions{}})
	if err := s.Delete(context.Background(), "u"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestGet_Profile(t *testing.T) {
	ctx := context.Background()
	u := &auth.User{
		ID: "u", Email: "a@b.c", Name: "Alice", EmailVerified: true,
		PlatformRole: auth.PlatformRoleUser, TOTPEnabled: true,
		AvatarKey: "avatars/u/x", CreatedAt: time.Unix(1000, 0),
	}
	s := New(Deps{Users: &fakeUsers{user: u}, Avatars: fakeAvatars{}})

	p, err := s.Get(ctx, "u")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Email != "a@b.c" || p.Name != "Alice" || !p.EmailVerified || !p.TOTPEnabled {
		t.Fatalf("profile mismatch: %+v", p)
	}
	if p.AvatarURL != "https://minio/get" {
		t.Fatalf("avatar url = %q, want presigned GET", p.AvatarURL)
	}
}
