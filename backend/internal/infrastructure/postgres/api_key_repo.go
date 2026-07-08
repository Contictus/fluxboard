package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/apikey"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// APIKeyRepo is the Postgres-backed apikey.APIKeyRepository. api_keys is [T] (RLS),
// so the org-scoped operations (Create/List/Revoke) run through TenantPool. The
// auth-path lookups (GetByHash/TouchLastUsed) run BEFORE any tenant context exists,
// so they use the owner pool, which bypasses the ENABLE (non-FORCE) RLS on api_keys
// — the key_hash is globally unique and unguessable, making that read safe.
type APIKeyRepo struct {
	tp   *TenantPool
	auth *gen.Queries // owner pool, RLS-bypassing (auth path only)
}

// NewAPIKeyRepo builds the repo. ownerPool must be an RLS-bypassing (table owner)
// pool used only for the by-hash auth lookups.
func NewAPIKeyRepo(tp *TenantPool, ownerPool *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{tp: tp, auth: gen.New(ownerPool)}
}

var _ apikey.APIKeyRepository = (*APIKeyRepo)(nil)

// Create inserts a new key in the org's tenant tx.
func (r *APIKeyRepo) Create(ctx context.Context, orgID string, k apikey.APIKey) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		kid, err := parseUUID(k.ID)
		if err != nil {
			return err
		}
		createdBy, err := nullableUUID(ptrOrNil(k.CreatedBy))
		if err != nil {
			return err
		}
		return q.CreateAPIKey(ctx, gen.CreateAPIKeyParams{
			ID:        kid,
			OrgID:     oid,
			Prefix:    k.Prefix,
			KeyHash:   k.KeyHash,
			Name:      k.Name,
			Scopes:    scopesToStrings(k.Scopes),
			CreatedBy: createdBy,
		})
	})
}

// List returns the org's keys, newest first.
func (r *APIKeyRepo) List(ctx context.Context, orgID string) ([]apikey.APIKey, error) {
	var out []apikey.APIKey
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListAPIKeysByOrg(ctx, oid)
		if err != nil {
			return err
		}
		out = make([]apikey.APIKey, 0, len(rows))
		for _, row := range rows {
			out = append(out, apiKeyFromRow(row))
		}
		return nil
	})
	return out, err
}

// GetByHash resolves a key by its SHA-256 hash on the auth (owner) pool.
func (r *APIKeyRepo) GetByHash(ctx context.Context, keyHash string) (apikey.APIKey, error) {
	row, err := r.auth.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apikey.APIKey{}, domain.ErrNotFound
		}
		return apikey.APIKey{}, err
	}
	return apiKeyFromRow(row), nil
}

// Revoke stamps revoked_at on one active key the org owns; ErrNotFound otherwise.
func (r *APIKeyRepo) Revoke(ctx context.Context, orgID, id string) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		kid, err := parseUUID(id)
		if err != nil {
			return err
		}
		rows, err := q.RevokeAPIKey(ctx, gen.RevokeAPIKeyParams{OrgID: oid, ID: kid})
		if err != nil {
			return err
		}
		if rows == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// TouchLastUsed advances last_used_at on the auth (owner) pool (best-effort).
func (r *APIKeyRepo) TouchLastUsed(ctx context.Context, keyHash string) error {
	return r.auth.TouchAPIKeyLastUsed(ctx, keyHash)
}

func apiKeyFromRow(row gen.ApiKey) apikey.APIKey {
	return apikey.APIKey{
		ID:         row.ID.String(),
		OrgID:      row.OrgID.String(),
		Prefix:     row.Prefix,
		KeyHash:    row.KeyHash,
		Name:       row.Name,
		Scopes:     scopesFromStrings(row.Scopes),
		CreatedBy:  derefStr(uuidStrPtr(row.CreatedBy)),
		LastUsedAt: tsPtr(row.LastUsedAt),
		RevokedAt:  tsPtr(row.RevokedAt),
		CreatedAt:  row.CreatedAt,
	}
}

func scopesToStrings(ss []apikey.Scope) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

func scopesFromStrings(ss []string) []apikey.Scope {
	out := make([]apikey.Scope, len(ss))
	for i, s := range ss {
		out[i] = apikey.Scope(s)
	}
	return out
}
