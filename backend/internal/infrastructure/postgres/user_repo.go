package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// uniqueViolation is Postgres SQLSTATE 23505.
const uniqueViolation = "23505"

// isUnique reports whether err is a unique-constraint violation.
func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// UserRepo is the Postgres-backed auth.UserRepository. Users are global (no RLS),
// so it uses the raw pool rather than the tenant-scoped connection.
type UserRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewUserRepo builds a UserRepo over the given pool.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool, q: gen.New(pool)}
}

var _ auth.UserRepository = (*UserRepo)(nil)

func (r *UserRepo) Create(ctx context.Context, u *auth.User) error {
	id, err := parseUUID(u.ID)
	if err != nil {
		return fmt.Errorf("user create: %w", err)
	}
	err = r.q.CreateUser(ctx, gen.CreateUserParams{
		ID:            id,
		Email:         u.Email,
		PasswordHash:  u.PasswordHash, // "" -> nullif -> NULL for OAuth-only
		Name:          u.Name,
		EmailVerified: u.EmailVerified,
		PlatformRole:  string(u.PlatformRole),
	})
	if isUnique(err) {
		return domain.ErrConflict // email already registered
	}
	return err
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("user by email: %w", err)
	}
	return &auth.User{
		ID:            row.ID.String(),
		Email:         row.Email,
		PasswordHash:  row.PasswordHash,
		Name:          row.Name,
		AvatarKey:     row.AvatarKey,
		EmailVerified: row.EmailVerified,
		PlatformRole:  auth.PlatformRole(row.PlatformRole),
		TOTPSecret:    row.TotpSecret,
		TOTPEnabled:   row.TotpEnabled,
		LockedAt:      tsPtr(row.LockedAt),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*auth.User, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("user by id: %w", err)
	}
	row, err := r.q.GetUserByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("user by id: %w", err)
	}
	return &auth.User{
		ID:            row.ID.String(),
		Email:         row.Email,
		PasswordHash:  row.PasswordHash,
		Name:          row.Name,
		AvatarKey:     row.AvatarKey,
		EmailVerified: row.EmailVerified,
		PlatformRole:  auth.PlatformRole(row.PlatformRole),
		TOTPSecret:    row.TotpSecret,
		TOTPEnabled:   row.TotpEnabled,
		LockedAt:      tsPtr(row.LockedAt),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}, nil
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return r.q.UpdateUserPasswordHash(ctx, gen.UpdateUserPasswordHashParams{
		PasswordHash: ptrOrNil(hash),
		ID:           uid,
	})
}

func (r *UserRepo) MarkEmailVerified(ctx context.Context, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("verify email: %w", err)
	}
	return r.q.MarkUserEmailVerified(ctx, uid)
}
