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
}

// SubtaskRepository persists subtasks (FR-TASK-003).
type SubtaskRepository interface {
	Create(ctx context.Context, orgID string, s *Subtask) error
	Get(ctx context.Context, orgID, id string) (*Subtask, error)
	ListByTask(ctx context.Context, orgID, taskID string) ([]Subtask, error)
	// Update sets title and done.
	Update(ctx context.Context, orgID, id, title string, done bool) error
	Delete(ctx context.Context, orgID, id string) error
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
}

// ActivityRepository appends and reads the per-task change log (FR-TASK-002).
type ActivityRepository interface {
	Append(ctx context.Context, orgID string, a *Activity) error
	// ListByTask returns a task's activity newest-first.
	ListByTask(ctx context.Context, orgID, taskID string) ([]Activity, error)
}
