package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
)

// LoginRateLimiter is a fixed-window counter over Redis implementing
// auth.LoginRateLimiter (docs/04-AUTH.md §4). The first attempt in a window sets
// the key with a TTL; each attempt INCRs it. Over the limit within the window
// is denied until the key expires. Fixed-window is deliberate: it is O(1), needs
// no stored timestamps, and a brief boundary burst is acceptable for a login
// throttle whose job is to blunt automated stuffing, not to be exact.
type LoginRateLimiter struct {
	rdb    *redis.Client
	limit  int64
	window time.Duration
}

// NewLoginRateLimiter builds a limiter allowing limit attempts per window.
func NewLoginRateLimiter(rdb *redis.Client, limit int64, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{rdb: rdb, limit: limit, window: window}
}

var _ auth.LoginRateLimiter = (*LoginRateLimiter)(nil)

// Allow increments the counter for key and reports whether it is within budget.
// The TTL is only (re)set on the first attempt so the window does not slide
// forward on every hit (which would let a steady attacker never expire it).
func (l *LoginRateLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	n, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, 0, err
	}
	if n == 1 {
		if err := l.rdb.Expire(ctx, key, l.window).Err(); err != nil {
			return false, 0, err
		}
	}
	if n > l.limit {
		ttl, err := l.rdb.TTL(ctx, key).Result()
		if err != nil || ttl < 0 {
			ttl = l.window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}

// Reset deletes the counter so a successful login clears prior failed attempts.
func (l *LoginRateLimiter) Reset(ctx context.Context, key string) error {
	return l.rdb.Del(ctx, key).Err()
}
