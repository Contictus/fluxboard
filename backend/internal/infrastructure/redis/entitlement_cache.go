package redisx

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// EntitlementCache implements billing.EntitlementCache over Redis (06 §7). The
// resolved entitlement envelope is cached at ent:{orgID} with a short TTL (60s);
// the webhook consumer and plan-change usecase Bust the key (write-through
// invalidation), so the TTL is only a safety net for a missed bust.
type EntitlementCache struct {
	rdb *redis.Client
}

// NewEntitlementCache builds an EntitlementCache over the given client.
func NewEntitlementCache(rdb *redis.Client) *EntitlementCache { return &EntitlementCache{rdb: rdb} }

var _ billing.EntitlementCache = (*EntitlementCache)(nil)

func entKey(orgID string) string { return "ent:" + orgID }

// Get returns cached entitlements; ok=false on a miss.
func (c *EntitlementCache) Get(ctx context.Context, orgID string) (billing.Entitlements, bool, error) {
	b, err := c.rdb.Get(ctx, entKey(orgID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return billing.Entitlements{}, false, nil
	}
	if err != nil {
		return billing.Entitlements{}, false, err
	}
	var e billing.Entitlements
	if err := json.Unmarshal(b, &e); err != nil {
		// Corrupt cache entry: treat as a miss so the caller re-derives.
		return billing.Entitlements{}, false, nil
	}
	return e, true, nil
}

// Set caches entitlements with ttl.
func (c *EntitlementCache) Set(ctx context.Context, orgID string, e billing.Entitlements, ttl time.Duration) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, entKey(orgID), b, ttl).Err()
}

// Bust deletes the cache entry.
func (c *EntitlementCache) Bust(ctx context.Context, orgID string) error {
	return c.rdb.Del(ctx, entKey(orgID)).Err()
}
