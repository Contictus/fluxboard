package apikey

import "context"

// APIKeyRepository persists org-scoped API keys. api_keys is tenant-owned ([T]);
// create/list/revoke run under a tenant tx (RLS, invariant #1). ResolveByKey is the
// exception: it runs on the AUTH path before any tenant context exists, so its
// implementation looks up by key_hash on the plain pool (the hash is unguessable
// and globally unique) and returns the owning org from the row.
type APIKeyRepository interface {
	// Create inserts a new key (hash + prefix already derived by the usecase).
	Create(ctx context.Context, orgID string, k APIKey) error
	// List returns an org's keys, newest first (secrets never included).
	List(ctx context.Context, orgID string) ([]APIKey, error)
	// GetByHash resolves a key by its SHA-256 hash for authentication. Returns
	// domain.ErrNotFound when absent. Revoked keys ARE returned (the caller checks
	// Active) so a revoked key yields 401, not a silent miss.
	GetByHash(ctx context.Context, keyHash string) (APIKey, error)
	// Revoke stamps revoked_at on one key the org owns; ErrNotFound if absent.
	Revoke(ctx context.Context, orgID, id string) error
	// TouchLastUsed advances last_used_at (best-effort; bounded write rate).
	TouchLastUsed(ctx context.Context, keyHash string) error
}
