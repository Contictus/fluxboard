package redisx

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// MembershipCache implements tenant.MembershipCache over Redis. It lets the
// TenantResolver middleware resolve the caller's org role without a Postgres
// round trip on every request, while bounding authorization-revocation latency
// to the TTL: a removed/demoted member's cached role is invalidated on change
// and otherwise expires within the TTL (docs/05-TENANCY-RBAC.md §6, ≤60s).
type MembershipCache struct {
	rdb *redis.Client
}

// NewMembershipCache builds a MembershipCache over the given client.
func NewMembershipCache(rdb *redis.Client) *MembershipCache { return &MembershipCache{rdb: rdb} }

var _ tenant.MembershipCache = (*MembershipCache)(nil)

func memberKey(orgID, userID string) string { return "member:" + orgID + ":" + userID }

// GetRole returns (role, known). A miss is ("", false).
func (c *MembershipCache) GetRole(ctx context.Context, orgID, userID string) (tenant.OrgRole, bool, error) {
	v, err := c.rdb.Get(ctx, memberKey(orgID, userID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return tenant.OrgRole(v), true, nil
}

// SetRole caches the resolved role for ttl.
func (c *MembershipCache) SetRole(ctx context.Context, orgID, userID string, role tenant.OrgRole, ttl time.Duration) error {
	return c.rdb.Set(ctx, memberKey(orgID, userID), string(role), ttl).Err()
}

// Invalidate drops the cached role so the next request re-resolves.
func (c *MembershipCache) Invalidate(ctx context.Context, orgID, userID string) error {
	return c.rdb.Del(ctx, memberKey(orgID, userID)).Err()
}
