package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// TokenRepo is the Postgres-backed auth.OneTimeTokenRepository.
type TokenRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewTokenRepo builds a TokenRepo over the given pool.
func NewTokenRepo(pool *pgxpool.Pool) *TokenRepo {
	return &TokenRepo{pool: pool, q: gen.New(pool)}
}

var _ auth.OneTimeTokenRepository = (*TokenRepo)(nil)

func (r *TokenRepo) Create(ctx context.Context, t *auth.OneTimeToken) error {
	id, err := parseUUID(t.ID)
	if err != nil {
		return fmt.Errorf("ott create: %w", err)
	}
	uid, err := parseUUID(t.UserID)
	if err != nil {
		return fmt.Errorf("ott create: %w", err)
	}
	payload := []byte("{}")
	if t.Payload != nil {
		if payload, err = json.Marshal(t.Payload); err != nil {
			return fmt.Errorf("ott create: payload: %w", err)
		}
	}
	return r.q.CreateOneTimeToken(ctx, gen.CreateOneTimeTokenParams{
		ID:        id,
		Purpose:   string(t.Purpose),
		UserID:    uid,
		TokenHash: t.TokenHash,
		Payload:   payload,
		ExpiresAt: t.ExpiresAt,
	})
}

func (r *TokenRepo) Consume(ctx context.Context, purpose auth.TokenPurpose, hash []byte, _ time.Time) (*auth.OneTimeToken, error) {
	row, err := r.q.ConsumeOneTimeToken(ctx, gen.ConsumeOneTimeTokenParams{
		Purpose:   string(purpose),
		TokenHash: hash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound // missing / expired / already used
		}
		return nil, fmt.Errorf("ott consume: %w", err)
	}
	var payload map[string]any
	if len(row.Payload) > 0 {
		_ = json.Unmarshal(row.Payload, &payload)
	}
	return &auth.OneTimeToken{
		ID:        row.ID.String(),
		Purpose:   auth.TokenPurpose(row.Purpose),
		UserID:    row.UserID.String(),
		TokenHash: row.TokenHash,
		Payload:   payload,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    tsPtr(row.UsedAt),
		CreatedAt: row.CreatedAt,
	}, nil
}
