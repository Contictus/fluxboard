package project

import (
	"context"
	"time"
)

// All repositories persist tenant-owned ([T]) data: every method takes orgID
// first and the implementation opens a tenant-scoped transaction (SET LOCAL
// app.current_tenant) so RLS is the backstop under the org_id filter. Reads
// return domain.ErrNotFound when absent; writes return domain.ErrConflict on a
// uniqueness clash.

// ProjectRepository persists projects.
type ProjectRepository interface {
	// Create inserts a project. domain.ErrConflict on (org_id, key) clash.
	Create(ctx context.Context, orgID string, p *Project) error
	// Get returns a project by id, or domain.ErrNotFound.
	Get(ctx context.Context, orgID, id string) (*Project, error)
	// List returns projects visible to userID. seeAll (org ADMIN+) returns every
	// project; otherwise 'org'-visible projects plus 'private' ones the user is a
	// member of. includeArchived toggles archived projects (FR-PROJ-002/006).
	List(ctx context.Context, orgID, userID string, seeAll, includeArchived bool) ([]Project, error)
	// Update sets name, description, color and visibility.
	Update(ctx context.Context, orgID string, p *Project) error
	// SetArchived sets or clears archived_at (FR-PROJ-006).
	SetArchived(ctx context.Context, orgID, id string, archivedAt *time.Time) error
	// NextNumber atomically increments the project's task counter and returns the
	// new value (per-project task number, FR-TASK-001).
	NextNumber(ctx context.Context, orgID, projectID string) (int, error)
}

// ProjectMemberRepository persists project membership + roles (FR-PROJ-003).
type ProjectMemberRepository interface {
	// Add upserts a member's role. domain.ErrNotFound if the project is gone.
	Add(ctx context.Context, orgID, projectID, userID string, role ProjectRole) error
	// Get returns the user's project membership, or domain.ErrNotFound.
	Get(ctx context.Context, orgID, projectID, userID string) (*ProjectMember, error)
	// List returns a project's members.
	List(ctx context.Context, orgID, projectID string) ([]ProjectMember, error)
	// Remove deletes a membership. domain.ErrNotFound if absent.
	Remove(ctx context.Context, orgID, projectID, userID string) error
}

// BoardRepository persists the default board per project (FR-PROJ-004).
type BoardRepository interface {
	Create(ctx context.Context, orgID string, b *Board) error
	// GetByProject returns the project's default board, or domain.ErrNotFound.
	GetByProject(ctx context.Context, orgID, projectID string) (*Board, error)
}

// ColumnRepository persists board columns (statuses).
type ColumnRepository interface {
	Create(ctx context.Context, orgID string, c *Column) error
	Get(ctx context.Context, orgID, id string) (*Column, error)
	// ListByBoard returns a board's columns ordered by rank.
	ListByBoard(ctx context.Context, orgID, boardID string) ([]Column, error)
	// Update sets name and wip_limit.
	Update(ctx context.Context, orgID, id, name string, wipLimit *int) error
	// SetRank moves a column (reorder, FR-PROJ-004).
	SetRank(ctx context.Context, orgID, id, rank string) error
	// Delete removes a column. Callers must migrate its tasks first.
	Delete(ctx context.Context, orgID, id string) error
	// CountTasks counts tasks in a column (delete guard / WIP warning).
	CountTasks(ctx context.Context, orgID, columnID string) (int, error)
}

// TaskRepository persists tasks.
type TaskRepository interface {
	// Create inserts a task. The implementation assigns t.Number from the
	// project counter inside the same transaction; t.Rank and t.ColumnID must be
	// set by the caller.
	Create(ctx context.Context, orgID string, t *Task) error
	// Get returns a task by id, or domain.ErrNotFound.
	Get(ctx context.Context, orgID, id string) (*Task, error)
	// ListByColumn returns a column's tasks ordered by rank.
	ListByColumn(ctx context.Context, orgID, columnID string) ([]Task, error)
	// ListByProject returns a project's tasks ordered by (column rank, task rank).
	ListByProject(ctx context.Context, orgID, projectID string) ([]Task, error)
	// Update writes editable fields (title, description, assignee, priority,
	// due_date).
	Update(ctx context.Context, orgID string, t *Task) error
	// Move relocates a task to (columnID, rank). domain.ErrConflict if the rank
	// is already taken in that column (concurrent move, FR-PROJ-005).
	Move(ctx context.Context, orgID, id, columnID, rank string) error

	// SoftDelete moves a task to Trash at deletedAt (FR-TASK-009). ErrNotFound if
	// the task is absent or already trashed.
	SoftDelete(ctx context.Context, orgID, id string, deletedAt time.Time) error
	// Restore returns a trashed task to its board. ErrNotFound if not trashed;
	// ErrConflict if its (column_id, rank) slot was reclaimed while trashed.
	Restore(ctx context.Context, orgID, id string) error
	// GetTrashed returns a trashed task by id, or ErrNotFound.
	GetTrashed(ctx context.Context, orgID, id string) (*Task, error)
	// ListTrashed returns a project's trashed tasks, newest-deleted first.
	ListTrashed(ctx context.Context, orgID, projectID string) ([]Task, error)
	// PurgeExpired hard-deletes tasks trashed before cutoff and returns the count
	// (nightly purge job, per-tenant, FR-TASK-009).
	PurgeExpired(ctx context.Context, orgID string, cutoff time.Time) (int, error)

	// Search runs a paginated full-text query with optional facet filters
	// (FR-TASK-007).
	Search(ctx context.Context, orgID string, f SearchFilter) (*SearchResult, error)

	// CountLive returns how many of ids are live tasks in the org (bulk pre-check).
	CountLive(ctx context.Context, orgID string, ids []string) (int, error)
	// BulkAssign sets assignee on every id in one transaction (nil clears it).
	BulkAssign(ctx context.Context, orgID string, ids []string, assigneeID *string) error
	// BulkMove relocates every ids[i] into columnID at ranks[i] (caller mints a
	// distinct rank per task), all in one transaction. ErrConflict on any rank
	// clash. len(ids) must equal len(ranks).
	BulkMove(ctx context.Context, orgID, columnID string, ids, ranks []string) error
}

// AttachmentRepository persists task attachments (FR-TASK-006).
type AttachmentRepository interface {
	// Create inserts a 'pending' attachment row.
	Create(ctx context.Context, orgID string, a *Attachment) error
	// Get returns an attachment by id, or ErrNotFound.
	Get(ctx context.Context, orgID, id string) (*Attachment, error)
	// Commit flips a pending row to committed with the HEAD-verified size.
	// ErrNotFound if absent or already committed.
	Commit(ctx context.Context, orgID, id string, sizeBytes int64, confirmedAt time.Time) error
	// ListByTask returns a task's committed attachments oldest-first.
	ListByTask(ctx context.Context, orgID, taskID string) ([]Attachment, error)
	// Delete removes an attachment row.
	Delete(ctx context.Context, orgID, id string) error
	// SumOrgBytes totals committed bytes for the org (quota check).
	SumOrgBytes(ctx context.Context, orgID string) (int64, error)
	// ListOrphans returns pending rows older than cutoff (orphan GC).
	ListOrphans(ctx context.Context, orgID string, cutoff time.Time) ([]Attachment, error)
}

// ObjectStore is the object-storage port (MinIO/S3) for attachment bytes. The
// API never proxies bytes: it hands clients presigned URLs (FR-TASK-006).
type ObjectStore interface {
	// PresignPut returns a time-limited upload URL for key.
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)
	// PresignGet returns a time-limited download URL for key, forcing a download
	// with the given filename.
	PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error)
	// Stat returns the stored object's size, or ErrNotFound if it is absent.
	Stat(ctx context.Context, key string) (int64, error)
	// Remove deletes the object (idempotent; missing object is not an error).
	Remove(ctx context.Context, key string) error
}

// SubtaskRepository persists subtasks (FR-TASK-003).
type SubtaskRepository interface {
	Create(ctx context.Context, orgID string, s *Subtask) error
	Get(ctx context.Context, orgID, id string) (*Subtask, error)
	ListByTask(ctx context.Context, orgID, taskID string) ([]Subtask, error)
	// Update sets title and done.
	Update(ctx context.Context, orgID, id, title string, done bool) error
	Delete(ctx context.Context, orgID, id string) error
	// CountForProject returns per-task subtask totals and done counts in one
	// query (board projection enrichment).
	CountForProject(ctx context.Context, orgID, projectID string) (map[string]SubtaskCount, error)
}

// SubtaskCount aggregates a task's checklist progress.
type SubtaskCount struct {
	Total int
	Done  int
}

// LabelRepository persists org-scoped labels and their task attachments
// (FR-TASK-004).
type LabelRepository interface {
	Create(ctx context.Context, orgID string, l *Label) error
	Get(ctx context.Context, orgID, id string) (*Label, error)
	List(ctx context.Context, orgID string) ([]Label, error)
	Update(ctx context.Context, orgID, id, name, color string) error
	// Delete removes a label; the FK cascade detaches it from every task.
	Delete(ctx context.Context, orgID, id string) error
	// Attach links a label to a task (idempotent).
	Attach(ctx context.Context, orgID, taskID, labelID string) error
	// Detach unlinks a label from a task.
	Detach(ctx context.Context, orgID, taskID, labelID string) error
	// ListForTask returns a task's labels.
	ListForTask(ctx context.Context, orgID, taskID string) ([]Label, error)
	// ListForProject returns every label attachment in a project, grouped by
	// task ID, in a single query (board projection enrichment).
	ListForProject(ctx context.Context, orgID, projectID string) (map[string][]Label, error)
}

// CommentRepository persists task comments (FR-TASK-005).
type CommentRepository interface {
	Create(ctx context.Context, orgID string, c *Comment) error
	Get(ctx context.Context, orgID, id string) (*Comment, error)
	// ListByTask returns a task's comments (including soft-deleted placeholders)
	// oldest-first.
	ListByTask(ctx context.Context, orgID, taskID string) ([]Comment, error)
	// Update rewrites the body and marks it edited.
	Update(ctx context.Context, orgID, id, body string) error
	// SoftDelete marks a comment deleted (keeps the row as a placeholder).
	SoftDelete(ctx context.Context, orgID, id string) error
	// CountForProject returns per-task live comment counts in one query
	// (board projection enrichment).
	CountForProject(ctx context.Context, orgID, projectID string) (map[string]int, error)
}

// TimeEntryRepository persists task timers and manual entries (FR-TIME).
type TimeEntryRepository interface {
	Create(ctx context.Context, orgID string, e *TimeEntry) error
	Get(ctx context.Context, orgID, id string) (*TimeEntry, error)
	// ListByTask returns a task's entries newest-first.
	ListByTask(ctx context.Context, orgID, taskID string) ([]TimeEntry, error)
	// ListRunning returns the caller's running timers newest-first.
	ListRunning(ctx context.Context, orgID, userID string) ([]TimeEntry, error)
	// Stop stamps ended_at. ErrNotFound if absent or already stopped.
	Stop(ctx context.Context, orgID, id string, endedAt time.Time) error
	// Delete removes an entry. ErrNotFound if absent.
	Delete(ctx context.Context, orgID, id string) error
}

// SprintRepository persists project sprints and task assignment (FR-SPRINT).
type SprintRepository interface {
	Create(ctx context.Context, orgID string, s *Sprint) error
	Get(ctx context.Context, orgID, id string) (*Sprint, error)
	// ListByProject returns sprints oldest-first.
	ListByProject(ctx context.Context, orgID, projectID string) ([]Sprint, error)
	// GetActive returns the active sprint. ErrNotFound when none.
	GetActive(ctx context.Context, orgID, projectID string) (*Sprint, error)
	Update(ctx context.Context, orgID string, s *Sprint) error
	// Delete removes a planned sprint. Tasks keep no dangling reference:
	// the FK sets their sprint_id NULL.
	Delete(ctx context.Context, orgID, id string) error
	// SetTaskSprint assigns (nil clears) a task's sprint. The sprint must
	// belong to the task's project; enforced by the caller.
	SetTaskSprint(ctx context.Context, orgID, taskID string, sprintID *string) error
	// ClearSprint unassigns every task of one sprint in a project.
	ClearSprint(ctx context.Context, orgID, projectID, sprintID string) error
}

// CustomFieldRepository persists project custom fields and task values
// (FR-FIELDS).
type CustomFieldRepository interface {
	Create(ctx context.Context, orgID string, f *CustomField) error
	Get(ctx context.Context, orgID, id string) (*CustomField, error)
	// ListByProject returns fields in position order.
	ListByProject(ctx context.Context, orgID, projectID string) ([]CustomField, error)
	Update(ctx context.Context, orgID string, f *CustomField) error
	// Delete removes a field; values cascade.
	Delete(ctx context.Context, orgID, id string) error
	// SetValue upserts a task's value. ErrNotFound if task or field absent.
	SetValue(ctx context.Context, orgID, taskID, fieldID string, v CustomValue) error
	// ValuesByTask returns a task's values in field order.
	ValuesByTask(ctx context.Context, orgID, taskID string) ([]CustomValue, error)
	// ClearValue removes one task value. ErrNotFound if absent.
	ClearValue(ctx context.Context, orgID, taskID, fieldID string) error
}

// ActivityRepository appends and reads the per-task change log (FR-TASK-002).
type ActivityRepository interface {
	Append(ctx context.Context, orgID string, a *Activity) error
	// ListByTask returns a task's activity newest-first.
	ListByTask(ctx context.Context, orgID, taskID string) ([]Activity, error)
}
