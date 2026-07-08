package audit

import (
	"context"
	"time"
)

// Severity classifies an audit event. 'security' rows are the high-signal set
// (auth reuse, credential changes, privilege changes) indexed separately for the
// SOC/monitoring read path (docs/07 §4).
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeveritySecurity Severity = "security"
)

// Action is the stable event verb. Namespaced by subject so the viewer can
// filter by prefix ('auth.', 'member.', 'invitation.', 'org.'). Extend this list
// as later phases add auditable events (project.*, billing.*, admin.*).
type Action string

const (
	// Auth (docs/04 §3/§6, FR-AUTH-006/010/014).
	ActionRefreshReuse   Action = "auth.refresh_reuse"
	ActionLoginFailed    Action = "auth.login_failed"
	ActionPasswordChange Action = "auth.password_change"
	ActionPasswordReset  Action = "auth.password_reset"
	ActionEmailChange    Action = "auth.email_change"
	ActionSessionRevoke  Action = "auth.session_revoke"

	// Membership & invitations (docs/05 §5, FR-TEN-004/005).
	ActionMemberRoleChange Action = "member.role_change"
	ActionMemberRemove     Action = "member.remove"
	ActionMemberLeave      Action = "member.leave"
	ActionInvitationCreate Action = "invitation.create"
	ActionInvitationAccept Action = "invitation.accept"
	ActionInvitationRevoke Action = "invitation.revoke"

	// Organization lifecycle (docs/05 §2, FR-TEN-007/008).
	ActionOrgSlugChange        Action = "org.slug_change"
	ActionOrgSoftDelete        Action = "org.soft_delete"
	ActionOrgRestore           Action = "org.restore"
	ActionOrgTransferOwnership Action = "org.transfer_ownership"

	// Platform admin (docs/build/PHASE-6 §5, FR-ADM-002/003/004/006). Impersonation
	// rows carry both identities (ImpersonatorUserID set); overrides/flags/retries
	// record the operator action on a tenant.
	ActionImpersonateStart Action = "admin.impersonate_start"
	ActionOverrideSet      Action = "admin.override_set"
	ActionOverrideDelete   Action = "admin.override_delete"
	ActionFlagSet          Action = "admin.flag_set"
	ActionWebhookRetry     Action = "admin.webhook_retry"

	// API keys (docs/build/PHASE-6 §5, FR-API-001).
	ActionAPIKeyCreate Action = "apikey.create"
	ActionAPIKeyRevoke Action = "apikey.revoke"
)

// Entry is one immutable audit record. Empty string IDs map to SQL NULL in the
// repo (org_id NULL = platform-level event). Metadata is free-form context
// (never secrets); it is stored as jsonb.
type Entry struct {
	OrgID              string
	ActorUserID        string
	ImpersonatorUserID string
	Action             Action
	TargetType         string
	TargetID           string
	Metadata           map[string]any
	IP                 string
	UserAgent          string
	Severity           Severity
	CreatedAt          time.Time // zero -> DB default now()
}

// Writer appends audit entries. Implemented by infrastructure/postgres over the
// plain pool (audit_log is not tenant-scoped). Callers treat writes as
// best-effort: a failed append is logged, never surfaced to the user.
type Writer interface {
	Append(ctx context.Context, e Entry) error
}

// ExportCap bounds an audit CSV export (FR-AUD-003); the reader never streams more
// than this many rows in one export.
const ExportCap = 10000

// Filter parameters an audit-log read (FR-AUD-003 org viewer, FR-ADM-005 global).
// OrgID scopes to one tenant; empty OrgID means "all orgs" and is ONLY valid on the
// platform-admin global path (the org viewer always sets it — the isolation
// backstop for a non-RLS table, docs/07 §4). Zero-value time bounds are ignored.
type Filter struct {
	OrgID    string
	Actor    string
	Action   Action
	Severity Severity
	Since    time.Time
	Until    time.Time
	Limit    int // capped at ExportCap by the reader
}

// Reader queries the append-only audit_log. audit_log is not tenant-scoped (RLS),
// so isolation for the org viewer is an explicit org_id filter in SQL (Filter.OrgID),
// NOT the RLS GUC — the reader must reject an empty OrgID from the org-scoped path.
type Reader interface {
	// List returns entries newest-first per the filter (bounded by Limit/ExportCap).
	List(ctx context.Context, f Filter) ([]Entry, error)
}
