package tenantuc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

type fakeLogoStore struct {
	puts map[string]string
	gets int
}

func (f *fakeLogoStore) PresignPut(_ context.Context, key, _ string, _ time.Duration) (string, error) {
	if f.puts == nil {
		f.puts = map[string]string{}
	}
	f.puts[key] = "put-url:" + key
	return "put-url:" + key, nil
}

func (f *fakeLogoStore) PresignGet(_ context.Context, key, _ string, _ time.Duration) (string, error) {
	f.gets++
	return "get-url:" + key, nil
}

func (f *fakeLogoStore) Stat(_ context.Context, _ string) (int64, error) { return 1, nil }
func (f *fakeLogoStore) Remove(_ context.Context, _ string) error        { return nil }

func logoTestService(store *fakeLogoStore) *Service {
	members := &fakeMembers{m: map[string]map[string]*tenant.Membership{}}
	orgs := &fakeOrgs{orgs: map[string]*tenant.Organization{}, members: members}
	return New(Deps{Orgs: orgs, Members: members, Logos: store, Now: func() time.Time { return testNow }})
}

func TestRequestLogoUpload_Validation(t *testing.T) {
	s := logoTestService(&fakeLogoStore{})
	if _, _, err := s.RequestLogoUpload(ctx(), "o1", "image/gif", 100); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("bad type: err = %v, want ErrValidation", err)
	}
	if _, _, err := s.RequestLogoUpload(ctx(), "o1", "image/png", 0); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("empty: err = %v, want ErrValidation", err)
	}
	if _, _, err := s.RequestLogoUpload(ctx(), "o1", "image/png", maxLogoBytes+1); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("oversize: err = %v, want ErrValidation", err)
	}
}

func TestRequestLogoUpload_DisabledWithoutStore(t *testing.T) {
	// No Logos dep at all (a typed-nil store would be non-nil as an
	// interface — the production guard checks a true nil).
	members := &fakeMembers{m: map[string]map[string]*tenant.Membership{}}
	orgs := &fakeOrgs{orgs: map[string]*tenant.Organization{}, members: members}
	s := New(Deps{Orgs: orgs, Members: members, Now: func() time.Time { return testNow }})
	if _, _, err := s.RequestLogoUpload(ctx(), "o1", "image/png", 100); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestRequestLogoUpload_HappyPath(t *testing.T) {
	store := &fakeLogoStore{}
	s := logoTestService(store)
	key, url, err := s.RequestLogoUpload(ctx(), "o1", "image/png", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "org-logos/o1/") {
		t.Errorf("key = %q, want org namespace prefix", key)
	}
	if url == "" {
		t.Error("empty upload URL")
	}
}

func TestCheckLogoKey(t *testing.T) {
	if err := checkLogoKey("o1", "org-logos/o1/abc"); err != nil {
		t.Errorf("own key: %v", err)
	}
	for _, bad := range []string{"org-logos/o2/abc", "avatars/u1/abc", "", "org-logos/o1"} {
		if err := checkLogoKey("o1", bad); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("key %q: err = %v, want ErrValidation", bad, err)
		}
	}
}

func TestGetOrg_MintsLogoURL(t *testing.T) {
	store := &fakeLogoStore{}
	members := &fakeMembers{m: map[string]map[string]*tenant.Membership{}}
	orgs := &fakeOrgs{orgs: map[string]*tenant.Organization{
		"o1": {ID: "o1", Slug: "org1", Name: "Org", LogoKey: "org-logos/o1/abc"},
	}, members: members}
	s := New(Deps{Orgs: orgs, Members: members, Logos: store, Now: func() time.Time { return testNow }})
	got, err := s.GetOrg(context.Background(), "o1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LogoURL == "" {
		t.Error("expected a minted logo URL")
	}
	if store.gets != 1 {
		t.Errorf("presign gets = %d, want 1", store.gets)
	}
}
