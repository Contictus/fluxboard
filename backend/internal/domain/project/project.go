// Package project models the Phase 3 core domain: Project, Board, Column, Task,
// Subtask, Label, Comment and the per-task activity log, plus their ports.
// Stdlib-only; no external deps per ADR-001 (docs/03-ARCHITECTURE.md ADR-001). Ordering keys are computed by
// internal/pkg/rank (ADR-009); tenant isolation is enforced one layer down by
// RLS on org_id.
package project

import (
	"regexp"
	"time"
)

// ---- Enums ----------------------------------------------------------------

// ProjectRole is a member's role within a project (FR-PROJ-003). Ordered
// LEAD > CONTRIBUTOR > VIEWER. Org ADMIN+ hold an implicit LEAD (resolved in the
// usecase layer, not stored).
type ProjectRole string

const (
	RoleLead        ProjectRole = "LEAD"
	RoleContributor ProjectRole = "CONTRIBUTOR"
	RoleViewer      ProjectRole = "VIEWER"
)

func (r ProjectRole) rank() int {
	switch r {
	case RoleLead:
		return 2
	case RoleContributor:
		return 1
	case RoleViewer:
		return 0
	default:
		return -1
	}
}

// Valid reports whether r is a known project role.
func (r ProjectRole) Valid() bool { return r.rank() >= 0 }

// AtLeast reports whether r is at least as privileged as min.
func (r ProjectRole) AtLeast(min ProjectRole) bool { return r.rank() >= min.rank() }

// Visibility controls who can see a project (FR-PROJ-002).
type Visibility string

const (
	VisibilityOrg     Visibility = "org"     // all org members
	VisibilityPrivate Visibility = "private" // only project members
)

// Valid reports whether v is a known visibility.
func (v Visibility) Valid() bool { return v == VisibilityOrg || v == VisibilityPrivate }

// Priority is a task's priority (FR-TASK-001).
type Priority string

const (
	PriorityUrgent Priority = "urgent"
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
	PriorityNone   Priority = "none"
)

// Valid reports whether p is a known priority.
func (p Priority) Valid() bool {
	switch p {
	case PriorityUrgent, PriorityHigh, PriorityMedium, PriorityLow, PriorityNone:
		return true
	default:
		return false
	}
}

// ---- Validation constants -------------------------------------------------

const (
	// MaxTitleLen bounds a task title (FR-TASK-001).
	MaxTitleLen = 200
	// MaxLabelLen bounds a label name (FR-TASK-004).
	MaxLabelLen = 30
	// CommentEditWindow is how long after posting a comment may still be edited
	// (FR-TASK-005).
	CommentEditWindow = 15 * time.Minute

	// MaxAttachmentBytes caps a single upload at 25 MiB (FR-TASK-006).
	MaxAttachmentBytes int64 = 25 << 20
	// OrgStorageQuotaBytes caps total committed attachment storage per org. Plan
	// tiers refine this in Phase 4; this is the free-tier default (2 GiB).
	OrgStorageQuotaBytes int64 = 2 << 30
	// UploadURLTTL bounds a presigned PUT (client has this long to upload).
	UploadURLTTL = 15 * time.Minute
	// DownloadURLTTL bounds a presigned GET (FR-TASK-006: 5 min).
	DownloadURLTTL = 5 * time.Minute
	// OrphanGCAge is how long a 'pending' attachment survives before the nightly
	// GC reclaims it (client requested but never confirmed the upload).
	OrphanGCAge = 24 * time.Hour
	// TrashRetention is how long a soft-deleted task stays restorable before the
	// purge job hard-deletes it (FR-TASK-009: 30 days).
	TrashRetention = 30 * 24 * time.Hour

	// MaxBulkTasks caps a single bulk action's target set (FR-TASK-008).
	MaxBulkTasks = 100
	// DefaultSearchLimit / MaxSearchLimit bound a search page (FR-TASK-007).
	DefaultSearchLimit = 25
	// MaxSearchLimit is the page-size ceiling for search.
	MaxSearchLimit = 100
)

// AllowedAttachmentMIME is the upload MIME allowlist (FR-TASK-006). Anything not
// listed is rejected at request-upload time.
var AllowedAttachmentMIME = map[string]bool{
	"image/png":          true,
	"image/jpeg":         true,
	"image/gif":          true,
	"image/webp":         true,
	"application/pdf":     true,
	"text/plain":          true,
	"text/csv":            true,
	"application/zip":     true,
	"application/msword":  true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel":                                                true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
}

// AllowedMIME reports whether ct is an accepted attachment content type.
func AllowedMIME(ct string) bool { return AllowedAttachmentMIME[ct] }

// keyRe validates a project key: 2–6 uppercase ASCII letters (FR-PROJ-001).
var keyRe = regexp.MustCompile(`^[A-Z]{2,6}$`)

// ValidKey reports whether s is a well-formed project key.
func ValidKey(s string) bool { return keyRe.MatchString(s) }

// DefaultColumns are the seeded board statuses for a new project (FR-PROJ-004),
// in board order.
var DefaultColumns = []string{"Backlog", "Todo", "In Progress", "Done"}

// ---- Models ---------------------------------------------------------------

// Project is a board container scoped to an org (FR-PROJ-001).
type Project struct {
	ID          string
	OrgID       string
	Key         string
	Name        string
	Description string
	Color       string
	Visibility  Visibility
	ArchivedAt  *time.Time
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Archived reports whether the project is archived (read-only, FR-PROJ-006).
func (p *Project) Archived() bool { return p.ArchivedAt != nil }

// ProjectMember binds a user to a project with a project role.
type ProjectMember struct {
	ProjectID string
	UserID    string
	Role      ProjectRole
	CreatedAt time.Time
}

// Board is a project's default kanban board (FR-PROJ-004).
type Board struct {
	ID        string
	ProjectID string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Column is a board status column, ordered by Rank (FR-PROJ-004).
type Column struct {
	ID        string
	BoardID   string
	Name      string
	Rank      string
	WIPLimit  *int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Task is a work item on a board, ordered within its column by Rank.
type Task struct {
	ID          string
	OrgID       string
	ProjectID   string
	ColumnID    string
	Number      int
	Title       string
	Description string
	AssigneeID  *string
	Priority    Priority
	DueDate     *time.Time
	SprintID    *string
	Rank        string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Subtask is a flat one-level checklist item under a task (FR-TASK-003).
type Subtask struct {
	ID        string
	TaskID    string
	Title     string
	Done      bool
	Rank      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Label is an org-scoped tag (FR-TASK-004).
type Label struct {
	ID        string
	OrgID     string
	Name      string
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Comment is a markdown note on a task (FR-TASK-005). Deleted comments keep a
// row (soft delete) so the thread shows a placeholder.
type Comment struct {
	ID        string
	TaskID    string
	AuthorID  string
	Body      string
	Edited    bool
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TimeEntry is one timer run or manual entry on a task (FR-TIME). EndedAt
// nil means the timer is running.
type TimeEntry struct {
	ID        string
	OrgID     string
	TaskID    string
	UserID    string
	StartedAt time.Time
	EndedAt   *time.Time
	Note      string
	CreatedAt time.Time
}

// SprintStatus is a sprint's lifecycle state.
type SprintStatus string

const (
	SprintPlanned   SprintStatus = "planned"
	SprintActive    SprintStatus = "active"
	SprintCompleted SprintStatus = "completed"
)

// Sprint is a timeboxed iteration in a project (FR-SPRINT). Tasks join via
// Task.SprintID (nil = backlog). CompletedTotal/CompletedDone snapshot scope
// at completion for velocity.
type Sprint struct {
	ID             string
	OrgID          string
	ProjectID      string
	Name           string
	Goal           string
	Status         SprintStatus
	StartedAt      *time.Time
	EndedAt        *time.Time
	CompletedTotal int
	CompletedDone  int
	CreatedBy      string
	CreatedAt      time.Time
}

// ProjectForm is a shareable intake form for a project (ADR-023). Anyone with
// the unguessable token URL can file a task into TargetColumnID; only the
// token SHA-256 (TokenHash) is stored, so the raw token is shown once at
// create/rotate. TargetColumnID carries no DB FK (see 0027); the submit path
// validates column-in-project instead.
type ProjectForm struct {
	ID             string
	OrgID          string
	ProjectID      string
	Name           string
	Description    string
	TargetColumnID string
	TokenHash      []byte
	CreatedBy      string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MaxFormNameLen bounds the intake form title.
const MaxFormNameLen = 120

// Seconds returns the entry length against now for running timers.
func (e *TimeEntry) Seconds(now time.Time) int64 {
	end := now
	if e.EndedAt != nil {
		end = *e.EndedAt
	}
	s := int64(end.Sub(e.StartedAt).Seconds())
	if s < 0 {
		return 0
	}
	return s
}
// Deleted reports whether the comment is soft-deleted.
func (c *Comment) Deleted() bool { return c.DeletedAt != nil }

// Editable reports whether the comment may still be edited at time now
// (author check is the caller's responsibility).
func (c *Comment) Editable(now time.Time) bool {
	return !c.Deleted() && now.Sub(c.CreatedAt) <= CommentEditWindow
}

// Activity is one entry in a task's change history (FR-TASK-002).
type Activity struct {
	ID        string
	TaskID    string
	ActorID   string
	Field     string
	OldValue  *string
	NewValue  *string
	CreatedAt time.Time
}

// AttachmentStatus is the upload lifecycle state (FR-TASK-006).
type AttachmentStatus string

const (
	// AttachmentPending: the row + presigned PUT URL exist; the object may not.
	AttachmentPending AttachmentStatus = "pending"
	// AttachmentCommitted: the object is confirmed present and counts toward quota.
	AttachmentCommitted AttachmentStatus = "committed"
)

// Attachment is a MinIO-backed file on a task (FR-TASK-006). Its object lives in
// object storage; this row tracks the two-phase (pending → committed) lifecycle.
type Attachment struct {
	ID          string
	OrgID       string
	TaskID      string
	UploaderID  string
	ObjectKey   string
	Filename    string
	ContentType string
	SizeBytes   int64
	Status      AttachmentStatus
	CreatedAt   time.Time
	ConfirmedAt *time.Time
}

// SearchFilter carries the optional facets for a task search (FR-TASK-007). A
// zero-value field means "no filter on that facet".
type SearchFilter struct {
	Query      string
	UserID     string // the caller (for private-project visibility)
	SeeAll     bool   // caller is org ADMIN+ (sees every project)
	ProjectID  string
	AssigneeID string
	ColumnID   string
	Priority   string
	LabelID    string
	Limit      int
	Offset     int
}

// SearchResult is one page of task search hits plus the unpaged total.
type SearchResult struct {
	Tasks []Task
	Total int
}

// TaskLink is a directed dependency edge (FR-LINKS): TaskID is blocked by
// LinkedTaskID. Only the "blocks" relation exists in v1.
type TaskLink struct {
	TaskID       string
	LinkedTaskID string
	Title        string
	Number       int
	ColumnID     string
	ProjectID    string
	CreatedAt    time.Time
}

// Custom field types (FR-FIELDS).
const (
	FieldText   = "text"
	FieldNumber = "number"
	FieldDate   = "date"
	FieldSelect = "select"
)

// ValidFieldType reports whether t is a known custom-field type.
func ValidFieldType(t string) bool {
	switch t {
	case FieldText, FieldNumber, FieldDate, FieldSelect:
		return true
	}
	return false
}

// CustomField is a project-scoped typed attribute (FR-FIELDS). Options holds
// the select choices.
type CustomField struct {
	ID        string
	OrgID     string
	ProjectID string
	Name      string
	Type      string
	Options   []string
	Position  int
	CreatedBy string
	CreatedAt time.Time
}

// CustomValue is one task's value for a field. Only the column matching the
// field type is set.
type CustomValue struct {
	FieldID string
	Name    string
	Type    string
	Text    *string
	Number  *float64
	Date    *time.Time
}
