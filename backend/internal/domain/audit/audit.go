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
