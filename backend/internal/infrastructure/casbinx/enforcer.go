// Package casbinx wires the Casbin authorization enforcer: the embedded model
// and the static role→permission policy that mirrors the docs/05 §2 matrix.
// The enforcer answers the coarse org-role gate; the user↔role↔org mapping
// lives in the memberships table (resolved by TenantResolver), not in Casbin
// grouping policies — see ADR-013.
package casbinx

import (
	_ "embed"
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"

	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

//go:embed model.conf
var modelConf string

// roleSub renders a role as its Casbin subject token.
func roleSub(r tenant.OrgRole) string { return "role:" + string(r) }

// policies is the static permission matrix (docs/05 §2). Each row is stated at
// the lowest role that holds it; role inheritance grants it upward.
var policies = [][]string{
	{roleSub(tenant.RoleGuest), tenant.ObjOrg, tenant.ActRead},
	{roleSub(tenant.RoleMember), tenant.ObjProjects, tenant.ActWrite},
	{roleSub(tenant.RoleMember), tenant.ObjLabels, tenant.ActWrite},
	{roleSub(tenant.RoleAdmin), tenant.ObjOrg, tenant.ActWrite},
	{roleSub(tenant.RoleAdmin), tenant.ObjMembers, tenant.ActWrite},
	{roleSub(tenant.RoleAdmin), tenant.ObjInvitations, tenant.ActWrite},
	{roleSub(tenant.RoleAdmin), tenant.ObjBilling, tenant.ActRead},
	{roleSub(tenant.RoleAdmin), tenant.ObjBilling, tenant.ActWrite},
	{roleSub(tenant.RoleAdmin), tenant.ObjAPIKeys, tenant.ActWrite},
	{roleSub(tenant.RoleAdmin), tenant.ObjAudit, tenant.ActRead},
	{roleSub(tenant.RoleOwner), tenant.ObjOwnership, tenant.ActWrite},
}

// roleHierarchy: higher role inherits the lower role's permissions.
var roleHierarchy = [][]string{
	{roleSub(tenant.RoleOwner), roleSub(tenant.RoleAdmin)},
	{roleSub(tenant.RoleAdmin), roleSub(tenant.RoleMember)},
	{roleSub(tenant.RoleMember), roleSub(tenant.RoleGuest)},
}

// Enforcer is the Casbin-backed tenant.Authorizer.
type Enforcer struct {
	e *casbin.Enforcer
}

var _ tenant.Authorizer = (*Enforcer)(nil)

// New builds the enforcer from the embedded model and static policy. Policies
// are held in memory (no adapter): they are compiled-in constants, so there is
// no dual-write consistency concern.
func New() (*Enforcer, error) {
	m, err := model.NewModelFromString(modelConf)
	if err != nil {
		return nil, fmt.Errorf("casbin model: %w", err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("casbin enforcer: %w", err)
	}
	if _, err := e.AddPolicies(policies); err != nil {
		return nil, fmt.Errorf("casbin add policies: %w", err)
	}
	if _, err := e.AddGroupingPolicies(roleHierarchy); err != nil {
		return nil, fmt.Errorf("casbin add role hierarchy: %w", err)
	}
	return &Enforcer{e: e}, nil
}

// Allowed reports whether an org role may perform action on object.
func (x *Enforcer) Allowed(role tenant.OrgRole, object, action string) (bool, error) {
	return x.e.Enforce(roleSub(role), object, action)
}
