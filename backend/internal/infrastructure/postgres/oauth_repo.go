package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// OAuthRepo is the Postgres-backed auth.OAuthIdentityRepository (Google, 04 §4).
type OAuthRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewOAuthRepo builds an OAuthRepo over the given pool.
func NewOAuthRepo(pool *pgxpool.Pool) *OAuthRepo {
	return &OAuthRepo{pool: pool, q: gen.New(pool)}
}

var _ auth.OAuthIdentityRepository = (*OAuthRepo)(nil)

func (r *OAuthRepo) GetByProviderSub(ctx context.Context, provider, sub string) (*auth.OAuthIdentity, error) {
	row, err := r.q.GetOAuthIdentity(ctx, gen.GetOAuthIdentityParams{Provider: provider, ProviderSub: sub})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("oauth identity: %w", err)
	}
	return &auth.OAuthIdentity{
		ID:          row.ID.String(),
		UserID:      row.UserID.String(),
		Provider:    row.Provider,
		ProviderSub: row.ProviderSub,
		CreatedAt:   row.CreatedAt,
	}, nil
}

func (r *OAuthRepo) Create(ctx context.Context, oi *auth.OAuthIdentity) error {
	id, err := parseUUID(oi.ID)
	if err != nil {
		return fmt.Errorf("oauth create: %w", err)
	}
	uid, err := parseUUID(oi.UserID)
	if err != nil {
		return fmt.Errorf("oauth create: %w", err)
	}
	err = r.q.CreateOAuthIdentity(ctx, gen.CreateOAuthIdentityParams{
		ID:          id,
		UserID:      uid,
		Provider:    oi.Provider,
		ProviderSub: oi.ProviderSub,
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}
