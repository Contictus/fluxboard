package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// RecoveryRepo is the Postgres-backed auth.RecoveryCodeRepository. Codes are
// stored hashed and consumed single-use (docs/04-AUTH.md §4).
type RecoveryRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewRecoveryRepo builds a RecoveryRepo over the given pool.
func NewRecoveryRepo(pool *pgxpool.Pool) *RecoveryRepo {
	return &RecoveryRepo{pool: pool, q: gen.New(pool)}
}

var _ auth.RecoveryCodeRepository = (*RecoveryRepo)(nil)

// Replace swaps the user's codes for a fresh batch in one transaction so a
// partial regenerate never leaves a mix of old and new codes.
func (r *RecoveryRepo) Replace(ctx context.Context, userID string, codeHashes [][]byte) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("recovery replace: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("recovery replace: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	if err := q.DeleteUserRecoveryCodes(ctx, uid); err != nil {
		return fmt.Errorf("recovery replace: delete: %w", err)
	}
	for _, h := range codeHashes {
		if err := q.CreateRecoveryCode(ctx, gen.CreateRecoveryCodeParams{UserID: uid, CodeHash: h}); err != nil {
			return fmt.Errorf("recovery replace: insert: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("recovery replace: commit: %w", err)
	}
	return nil
}

func (r *RecoveryRepo) Consume(ctx context.Context, userID string, codeHash []byte) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, fmt.Errorf("recovery consume: %w", err)
	}
	n, err := r.q.ConsumeRecoveryCode(ctx, gen.ConsumeRecoveryCodeParams{UserID: uid, CodeHash: codeHash})
	if err != nil {
		return false, fmt.Errorf("recovery consume: %w", err)
	}
	return n > 0, nil
}
