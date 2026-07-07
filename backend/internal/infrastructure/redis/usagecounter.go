package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// UsageCounter records per-org request-path usage in Redis (docs/06-BILLING.md
// §5) and backs the plan-tier rate limiter (FR-BILL-009). Counters are lossy by
// design — the hourly usage:aggregate job snapshots them into usage_records,
// which is the durable store. Keys carry a 48h TTL so yesterday's values stay
// readable for a late aggregate run but nothing accumulates.
//
//	usage:{org}:api:{yyyymmdd}  INCR    — API calls that day
//	usage:{org}:am:{yyyymmdd}   PFADD   — HyperLogLog of active member user ids
//	rl:{org}:{unixMinute}       INCR    — fixed-window rate-limit counter
type UsageCounter struct {
	rdb *redis.Client
}

// NewUsageCounter builds a UsageCounter over the given client.
func NewUsageCounter(rdb *redis.Client) *UsageCounter { return &UsageCounter{rdb: rdb} }

const usageKeyTTL = 48 * time.Hour

func apiKey(orgID string, day time.Time) string {
	return "usage:" + orgID + ":api:" + day.UTC().Format("20060102")
}

func amKey(orgID string, day time.Time) string {
	return "usage:" + orgID + ":am:" + day.UTC().Format("20060102")
}

// RecordRequest counts one authenticated API call: INCRs the org's daily api
// counter and PFADDs the user into the daily active-member HyperLogLog, in one
// pipeline round trip.
func (c *UsageCounter) RecordRequest(ctx context.Context, orgID, userID string, now time.Time) error {
	pipe := c.rdb.Pipeline()
	ak, mk := apiKey(orgID, now), amKey(orgID, now)
	pipe.Incr(ctx, ak)
	pipe.Expire(ctx, ak, usageKeyTTL)
	pipe.PFAdd(ctx, mk, userID)
	pipe.Expire(ctx, mk, usageKeyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// Allow applies the plan's per-minute fixed-window rate limit: INCR the current
// minute's counter and report whether it is still within limit. The key expires
// after two windows so abandoned minutes clean themselves up. limit <= 0 is the
// caller's job to treat as unlimited (this method assumes a positive limit).
func (c *UsageCounter) Allow(ctx context.Context, orgID string, limit int, now time.Time) (bool, error) {
	key := fmt.Sprintf("rl:%s:%d", orgID, now.Unix()/60)
	pipe := c.rdb.Pipeline()
	n := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return n.Val() <= int64(limit), nil
}

// APICalls returns the org's api_calls counter for the given UTC day (0 when
// the key never existed — no traffic).
func (c *UsageCounter) APICalls(ctx context.Context, orgID string, day time.Time) (int64, error) {
	n, err := c.rdb.Get(ctx, apiKey(orgID, day)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return n, err
}

// ActiveMembers returns the distinct active-member estimate for the given UTC
// day (PFCOUNT; 0 for a missing key).
func (c *UsageCounter) ActiveMembers(ctx context.Context, orgID string, day time.Time) (int64, error) {
	return c.rdb.PFCount(ctx, amKey(orgID, day)).Result()
}
