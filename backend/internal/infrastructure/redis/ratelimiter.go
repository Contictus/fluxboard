package redisx

import (
	"context"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
)

// LoginRateLimiter is a Redis sorted-set sliding-window limiter implementing
// auth.LoginRateLimiter (docs/04-AUTH.md §2/§4). Each attempt is a member scored
// by its timestamp; on every call we evict members older than the window, add the
// current attempt, and count what remains. Unlike a fixed window this has no
// boundary burst — the budget always covers the trailing `window`.
//
// Scores are UnixMilli (≤ ~1.7e12), comfortably inside float64's exact-integer
// range, so ZSET scores stay precise (nanoseconds would not).
type LoginRateLimiter struct {
	rdb    *redis.Client
	limit  int64
	window time.Duration
}

// NewLoginRateLimiter builds a limiter allowing limit attempts per sliding window.
func NewLoginRateLimiter(rdb *redis.Client, limit int64, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{rdb: rdb, limit: limit, window: window}
}

var _ auth.LoginRateLimiter = (*LoginRateLimiter)(nil)

// Allow records an attempt for key and reports whether it is within budget. When
// denied it returns the time until the oldest in-window attempt ages out (the
// Retry-After hint).
func (l *LoginRateLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	now := time.Now()
	nowMs := now.UnixMilli()
	windowStart := now.Add(-l.window).UnixMilli()
	// Unique member so two attempts in the same millisecond do not collide.
	member := strconv.FormatInt(nowMs, 10) + ":" + strconv.FormatUint(rand.Uint64(), 36)

	pipe := l.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart-1, 10))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member})
	card := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, err
	}

	if card.Val() > l.limit {
		return false, l.retryAfter(ctx, key, now), nil
	}
	return true, 0, nil
}

// retryAfter returns how long until the oldest in-window attempt expires, so the
// caller can surface a Retry-After. Falls back to the full window on any error.
func (l *LoginRateLimiter) retryAfter(ctx context.Context, key string, now time.Time) time.Duration {
	oldest, err := l.rdb.ZRangeWithScores(ctx, key, 0, 0).Result()
	if err != nil || len(oldest) == 0 {
		return l.window
	}
	oldestAt := time.UnixMilli(int64(oldest[0].Score))
	if r := l.window - now.Sub(oldestAt); r > 0 {
		return r
	}
	return time.Second // just aged out; nudge the caller to retry shortly
}

// Reset clears the window so a successful login forgets prior failed attempts.
func (l *LoginRateLimiter) Reset(ctx context.Context, key string) error {
	return l.rdb.Del(ctx, key).Err()
}
