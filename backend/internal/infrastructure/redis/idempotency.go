package redisx

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// IdempotencyStore backs the Idempotency-Key replay guard (docs/08 §4) over
// Redis: a key maps to the id of the entity created on the first request, so a
// retry returns that id instead of creating a duplicate. It satisfies
// tenantuc.IdempotencyStore structurally (wired in cmd/api), mirroring how the
// mailer implements its usecase interface without importing the usecase package.
type IdempotencyStore struct {
	rdb *redis.Client
}

// NewIdempotencyStore builds an IdempotencyStore over the given client.
func NewIdempotencyStore(rdb *redis.Client) *IdempotencyStore { return &IdempotencyStore{rdb: rdb} }

// Get returns the stored value, or "" (no error) when the key is absent.
func (s *IdempotencyStore) Get(ctx context.Context, key string) (string, error) {
	v, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// Set stores val under key for ttl.
func (s *IdempotencyStore) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, val, ttl).Err()
}
