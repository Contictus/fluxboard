package redisx

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
)

// SessionCache implements auth.SessionCache over Redis. It bounds revocation
// latency (ADR 008: ~60s cache) so the Auth middleware need not hit Postgres on
// every request, while a revoked session takes effect within the cache TTL.
type SessionCache struct {
	rdb *redis.Client
}

// NewSessionCache builds a SessionCache over the given client.
func NewSessionCache(rdb *redis.Client) *SessionCache { return &SessionCache{rdb: rdb} }

var _ auth.SessionCache = (*SessionCache)(nil)

const (
	activeVal  = "active"
	revokedVal = "revoked"
	// revokeTTL keeps the revoked marker long enough to outlive any still-valid
	// access token bearing this sid (access TTL is 15m).
	revokeTTL = 15 * time.Minute
)

func sessKey(sid string) string { return "sess:" + sid }

// MarkActive records sid as a live session for ttl.
func (c *SessionCache) MarkActive(ctx context.Context, sid string, ttl time.Duration) error {
	return c.rdb.Set(ctx, sessKey(sid), activeVal, ttl).Err()
}

// Status returns (active, known). A cache miss is (false, false) — the caller
// backfills from Postgres.
func (c *SessionCache) Status(ctx context.Context, sid string) (active, known bool, err error) {
	v, err := c.rdb.Get(ctx, sessKey(sid)).Result()
	if errors.Is(err, redis.Nil) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return v == activeVal, true, nil
}

// Revoke marks sid revoked so subsequent requests 401 immediately.
func (c *SessionCache) Revoke(ctx context.Context, sid string) error {
	return c.rdb.Set(ctx, sessKey(sid), revokedVal, revokeTTL).Err()
}
