package apikeyuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/apikey"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
)

// fakeKeyRepo is an in-memory APIKeyRepository keyed by hash.
type fakeKeyRepo struct {
	byHash  map[string]apikey.APIKey
	touched int
}

func newFakeKeyRepo() *fakeKeyRepo { return &fakeKeyRepo{byHash: map[string]apikey.APIKey{}} }

func (f *fakeKeyRepo) Create(_ context.Context, _ string, k apikey.APIKey) error {
	f.byHash[k.KeyHash] = k
	return nil
}
func (f *fakeKeyRepo) List(_ context.Context, orgID string) ([]apikey.APIKey, error) {
	var out []apikey.APIKey
	for _, k := range f.byHash {
		if k.OrgID == orgID {
			out = append(out, k)
		}
	}
	return out, nil
}
func (f *fakeKeyRepo) GetByHash(_ context.Context, keyHash string) (apikey.APIKey, error) {
	k, ok := f.byHash[keyHash]
	if !ok {
		return apikey.APIKey{}, domain.ErrNotFound
	}
	return k, nil
}
func (f *fakeKeyRepo) Revoke(_ context.Context, _, id string) error {
	for h, k := range f.byHash {
		if k.ID == id {
			now := time.Now()
			k.RevokedAt = &now
			f.byHash[h] = k
			return nil
		}
	}
	return domain.ErrNotFound
}
func (f *fakeKeyRepo) TouchLastUsed(_ context.Context, _ string) error { f.touched++; return nil }

type fakeAudit struct{ entries []audit.Entry }

func (f *fakeAudit) Append(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

func TestCreateReturnsPlaintextOnceAndAudits(t *testing.T) {
	repo := newFakeKeyRepo()
	aud := &fakeAudit{}
	svc := New(Deps{Keys: repo, Audit: aud})

	c, err := svc.Create(context.Background(), "org1", "ci", "u1", []apikey.Scope{apikey.ScopeRead})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.Plaintext == "" || !apikey.LooksLikeKey(c.Plaintext) {
		t.Fatalf("expected a well-formed plaintext, got %q", c.Plaintext)
	}
	// The stored row must carry only the hash, never the plaintext.
	stored := repo.byHash[apikey.Hash(c.Plaintext)]
	if stored.KeyHash != apikey.Hash(c.Plaintext) {
		t.Fatal("stored key hash mismatch")
	}
	if len(aud.entries) != 1 || aud.entries[0].Action != audit.ActionAPIKeyCreate {
		t.Fatalf("expected one apikey.create audit row, got %+v", aud.entries)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	svc := New(Deps{Keys: newFakeKeyRepo()})
	if _, err := svc.Create(context.Background(), "org1", "", "u1", []apikey.Scope{apikey.ScopeRead}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty name: want ErrValidation, got %v", err)
	}
	if _, err := svc.Create(context.Background(), "org1", "n", "u1", nil); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("no scopes: want ErrValidation, got %v", err)
	}
	if _, err := svc.Create(context.Background(), "org1", "n", "u1", []apikey.Scope{"bogus"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("bad scope: want ErrValidation, got %v", err)
	}
}

func TestResolveByKey(t *testing.T) {
	repo := newFakeKeyRepo()
	svc := New(Deps{Keys: repo})
	c, err := svc.Create(context.Background(), "org1", "ci", "u1", []apikey.Scope{apikey.ScopeWrite})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Active key resolves and touches last_used.
	k, err := svc.ResolveByKey(context.Background(), c.Plaintext)
	if err != nil {
		t.Fatalf("resolve active: %v", err)
	}
	if k.OrgID != "org1" || !k.HasScope(apikey.ScopeRead) { // write ⇒ read
		t.Fatalf("resolved key wrong: %+v", k)
	}
	if repo.touched != 1 {
		t.Fatalf("expected last_used touch, got %d", repo.touched)
	}

	// Malformed secret is unauthorized without a repo hit.
	if _, err := svc.ResolveByKey(context.Background(), "not-a-key"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("malformed: want ErrUnauthorized, got %v", err)
	}
	// Unknown key is unauthorized (not ErrNotFound — the auth path must not distinguish).
	unknown := apikey.Prefix + "00000000deadbeef"
	if _, err := svc.ResolveByKey(context.Background(), unknown); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("unknown: want ErrUnauthorized, got %v", err)
	}
	// Revoked key is unauthorized.
	if err := svc.Revoke(context.Background(), "org1", c.Key.ID, "u1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.ResolveByKey(context.Background(), c.Plaintext); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked: want ErrUnauthorized, got %v", err)
	}
}
