package taskuc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// fakeTasks captures the SearchFilter it receives so tests can assert clamping.
type fakeTasks struct {
	project.TaskRepository
	gotFilter project.SearchFilter
}

func (f *fakeTasks) Search(_ context.Context, _ string, filt project.SearchFilter) (*project.SearchResult, error) {
	f.gotFilter = filt
	return &project.SearchResult{}, nil
}

// fakeStore is a no-op ObjectStore so attachmentsEnabled() is true.
type fakeStore struct{}

func (fakeStore) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "http://put", nil
}
func (fakeStore) PresignGet(context.Context, string, string, time.Duration) (string, error) {
	return "http://get", nil
}
func (fakeStore) Stat(context.Context, string) (int64, error) { return 0, domain.ErrNotFound }
func (fakeStore) Remove(context.Context, string) error        { return nil }

func TestSearchValidationAndClamp(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTasks{}
	s := New(Deps{Tasks: ft})

	if _, err := s.Search(ctx, "o", "u", project.SearchFilter{Query: "  "}, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("empty query: err %v, want ErrValidation", err)
	}
	if _, err := s.Search(ctx, "o", "u", project.SearchFilter{Query: "x", Priority: "bogus"}, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("bad priority: err %v, want ErrValidation", err)
	}

	// Over-limit clamps to MaxSearchLimit; SeeAll set for org ADMIN+.
	if _, err := s.Search(ctx, "o", "u", project.SearchFilter{Query: "hi", Limit: 9999}, tenant.RoleAdmin); err != nil {
		t.Fatalf("search: %v", err)
	}
	if ft.gotFilter.Limit != project.MaxSearchLimit {
		t.Errorf("limit = %d, want clamp to %d", ft.gotFilter.Limit, project.MaxSearchLimit)
	}
	if !ft.gotFilter.SeeAll {
		t.Error("org admin should search with SeeAll=true")
	}
	if ft.gotFilter.UserID != "u" {
		t.Errorf("UserID = %q, want u", ft.gotFilter.UserID)
	}

	// Default limit when unset.
	_, _ = s.Search(ctx, "o", "u", project.SearchFilter{Query: "hi"}, tenant.RoleMember)
	if ft.gotFilter.Limit != project.DefaultSearchLimit {
		t.Errorf("default limit = %d, want %d", ft.gotFilter.Limit, project.DefaultSearchLimit)
	}
}

func TestBulkGuards(t *testing.T) {
	ctx := context.Background()
	s := New(Deps{})

	if err := s.BulkAssign(ctx, "o", "u", nil, nil, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("empty set: err %v, want ErrValidation", err)
	}
	too := make([]string, project.MaxBulkTasks+1)
	if err := s.BulkAssign(ctx, "o", "u", too, nil, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("oversized set: err %v, want ErrValidation", err)
	}
}

func TestAttachmentUploadGuards(t *testing.T) {
	ctx := context.Background()

	// Store unset ⇒ attachments disabled ⇒ ErrForbidden.
	disabled := New(Deps{})
	if _, _, err := disabled.RequestUpload(ctx, "o", "u", "t", "a.png", "image/png", 10, tenant.RoleMember); err != domain.ErrForbidden {
		t.Fatalf("disabled: err %v, want ErrForbidden", err)
	}

	// Enabled: validation runs before any repo access, so nil repos are fine.
	s := New(Deps{Store: fakeStore{}})
	cases := []struct {
		name, filename, ct string
		size               int64
	}{
		{"empty filename", "", "image/png", 10},
		{"zero size", "a.png", "image/png", 0},
		{"too big", "a.png", "image/png", project.MaxAttachmentBytes + 1},
		{"bad mime", "a.exe", "application/x-msdownload", 10},
	}
	for _, c := range cases {
		if _, _, err := s.RequestUpload(ctx, "o", "u", "t", c.filename, c.ct, c.size, tenant.RoleMember); err != domain.ErrValidation {
			t.Errorf("%s: err %v, want ErrValidation", c.name, err)
		}
	}
}

func TestObjectKeyShape(t *testing.T) {
	k := objectKey("org1", "task1", "att1", "../../etc/passwd")
	if !strings.HasPrefix(k, "orgs/org1/tasks/task1/att1/") {
		t.Fatalf("key %q missing scoped prefix", k)
	}
	if strings.Contains(k, "..") {
		t.Errorf("key %q must not retain path traversal", k)
	}
}
