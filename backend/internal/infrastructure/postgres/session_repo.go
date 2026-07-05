package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// SessionRepo is the Postgres-backed auth.SessionRepository. It owns the refresh
// rotation transaction (docs/04-AUTH.md §3), including reuse detection.
type SessionRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewSessionRepo builds a SessionRepo over the given pool.
func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{pool: pool, q: gen.New(pool)}
}

var _ auth.SessionRepository = (*SessionRepo)(nil)

func (r *SessionRepo) Create(ctx context.Context, s *auth.Session) error {
	id, err := parseUUID(s.ID)
	if err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	fam, err := parseUUID(s.FamilyID)
	if err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	uid, err := parseUUID(s.UserID)
	if err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	return r.q.CreateSession(ctx, gen.CreateSessionParams{
		ID:        id,
		FamilyID:  fam,
		UserID:    uid,
		TokenHash: s.TokenHash,
		UserAgent: s.UserAgent,
		Ip:        s.IP, // "" -> nullif -> NULL
		ExpiresAt: s.ExpiresAt,
	})
}

func (r *SessionRepo) GetByID(ctx context.Context, id string) (*auth.Session, error) {
	sid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("session by id: %w", err)
	}
	row, err := r.q.GetSessionByID(ctx, sid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("session by id: %w", err)
	}
	return &auth.Session{
		ID:           row.ID.String(),
		FamilyID:     row.FamilyID.String(),
		UserID:       row.UserID.String(),
		TokenHash:    row.TokenHash,
		UserAgent:    row.UserAgent,
		IP:           row.Ip,
		ExpiresAt:    row.ExpiresAt,
		RotatedAt:    tsPtr(row.RotatedAt),
		RevokedAt:    tsPtr(row.RevokedAt),
		RevokeReason: row.RevokeReason,
		CreatedAt:    row.CreatedAt,
		LastUsedAt:   row.LastUsedAt,
	}, nil
}

// TouchLastUsed advances a live session's last_used_at. Best-effort; the auth
// middleware calls it on the cache-miss backfill path (FR-AUTH-008). Not part of
// the domain SessionRepository port — the middleware type-asserts for it — so
// test fakes need not implement it.
func (r *SessionRepo) TouchLastUsed(ctx context.Context, id string) error {
	sid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return r.q.TouchSessionLastUsed(ctx, sid)
}

// Rotate runs steps 1–6 of docs/04-AUTH.md §3 in one serializable transaction.
func (r *SessionRepo) Rotate(ctx context.Context, presentedHash, newHash []byte, newID, userAgent, ip string, newExpiry time.Time) (*auth.Session, error) {
	newUUID, err := parseUUID(newID)
	if err != nil {
		return nil, fmt.Errorf("rotate: %w", err)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("rotate: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit
	q := gen.New(tx)

	row, err := q.GetSessionByTokenHashForUpdate(ctx, presentedHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUnauthorized // unknown token
		}
		return nil, fmt.Errorf("rotate: lookup: %w", err)
	}

	switch {
	case row.RevokedAt.Valid:
		return nil, domain.ErrUnauthorized

	case row.RotatedAt.Valid:
		// Reuse of an already-rotated token: revoke the whole family and commit
		// so the revocation persists, then report reuse.
		reason := "token_reuse"
		if err := q.RevokeSessionFamily(ctx, gen.RevokeSessionFamilyParams{
			Reason: &reason, FamilyID: row.FamilyID,
		}); err != nil {
			return nil, fmt.Errorf("rotate: revoke family: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("rotate: commit reuse revoke: %w", err)
		}
		return nil, auth.ErrRefreshReuse

	case row.ExpiresAt.Before(time.Now()):
		reason := "expired"
		if err := q.RevokeSessionByID(ctx, gen.RevokeSessionByIDParams{
			Reason: &reason, ID: row.ID,
		}); err != nil {
			return nil, fmt.Errorf("rotate: revoke expired: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("rotate: commit expire: %w", err)
		}
		return nil, domain.ErrUnauthorized
	}

	// Happy path: mark old rotated, insert the successor in the same family.
	if err := q.MarkSessionRotated(ctx, row.ID); err != nil {
		return nil, fmt.Errorf("rotate: mark rotated: %w", err)
	}
	if err := q.CreateSession(ctx, gen.CreateSessionParams{
		ID:        newUUID,
		FamilyID:  row.FamilyID,
		UserID:    row.UserID,
		TokenHash: newHash,
		UserAgent: userAgent,
		Ip:        ip,
		ExpiresAt: newExpiry,
	}); err != nil {
		return nil, fmt.Errorf("rotate: insert successor: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("rotate: commit: %w", err)
	}

	return &auth.Session{
		ID:        newID,
		FamilyID:  row.FamilyID.String(),
		UserID:    row.UserID.String(),
		TokenHash: newHash,
		UserAgent: userAgent,
		IP:        ip,
		ExpiresAt: newExpiry,
	}, nil
}

func (r *SessionRepo) RevokeByID(ctx context.Context, id, reason string) error {
	sid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return r.q.RevokeSessionByID(ctx, gen.RevokeSessionByIDParams{Reason: ptrOrNil(reason), ID: sid})
}

func (r *SessionRepo) RevokeAllForUser(ctx context.Context, userID, reason string) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("revoke all: %w", err)
	}
	return r.q.RevokeAllUserSessions(ctx, gen.RevokeAllUserSessionsParams{Reason: ptrOrNil(reason), UserID: uid})
}

func (r *SessionRepo) ListForUser(ctx context.Context, userID string) ([]*auth.Session, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	rows, err := r.q.ListActiveUserSessions(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]*auth.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, &auth.Session{
			ID:           row.ID.String(),
			FamilyID:     row.FamilyID.String(),
			UserID:       row.UserID.String(),
			TokenHash:    row.TokenHash,
			UserAgent:    row.UserAgent,
			IP:           row.Ip,
			ExpiresAt:    row.ExpiresAt,
			RotatedAt:    tsPtr(row.RotatedAt),
			RevokedAt:    tsPtr(row.RevokedAt),
			RevokeReason: row.RevokeReason,
			CreatedAt:    row.CreatedAt,
			LastUsedAt:   row.LastUsedAt,
		})
	}
	return out, nil
}

func (r *SessionRepo) RevokeByIDForUser(ctx context.Context, id, userID, reason string) error {
	sid, err := parseUUID(id)
	if err != nil {
		// A non-UUID id can never match a row; treat as not-found, not a 500.
		return domain.ErrNotFound
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	n, err := r.q.RevokeUserSessionByID(ctx, gen.RevokeUserSessionByIDParams{
		Reason: ptrOrNil(reason), ID: sid, UserID: uid,
	})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound // not owned, already revoked, or nonexistent
	}
	return nil
}
