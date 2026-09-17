// Package taskuc holds the application services for tasks, subtasks, labels,
// comments and the per-task activity log (docs/01 §TASK). Domain-only deps.
// Fine-grained authorization (project VIEWER/CONTRIBUTOR/LEAD, org ADMIN+ as
// implicit LEAD) is enforced here; the coarse org-role Casbin gate runs in the
// HTTP middleware (ADR-013).
package taskuc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/automation"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/rank"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// EntitlementResolver resolves an org's entitlements so the storage-quota check
// uses the plan's max_storage_bytes instead of the hardcoded Free ceiling
// (FR-BILL-009). Optional: nil falls back to project.OrgStorageQuotaBytes.
// Implemented by billinguc.Service.
type EntitlementResolver interface {
	Resolve(ctx context.Context, orgID string) (billing.Entitlements, error)
}

// Notifier performs notification fan-out for task events (comments/@mentions and
// assignment). Implemented by notifyuc.Service; nil disables fan-out (tests).
// Calls are best-effort side effects — the notifier logs and swallows its own
// errors so a fan-out failure never fails the originating task action.
type Notifier interface {
	FanOutComment(ctx context.Context, orgID, actorID, projectID, taskID, commentID, body string, commentTargets []string)
	NotifyAssigned(ctx context.Context, orgID, actorID, taskID, assigneeID, taskTitle string)
}

// Deps are the collaborators the service needs.
type Deps struct {
	Tasks        project.TaskRepository
	Subtasks     project.SubtaskRepository
	Labels       project.LabelRepository
	Comments     project.CommentRepository
	Activity     project.ActivityRepository
	Attachments  project.AttachmentRepository
	TimeEntries  project.TimeEntryRepository // timers; nil ⇒ time endpoints disabled
	Links        project.TaskLinkRepository  // dependencies; nil ⇒ link endpoints disabled
	Projects     project.ProjectRepository
	Members      project.ProjectMemberRepository
	Boards       project.BoardRepository
	Columns      project.ColumnRepository
	Store        project.ObjectStore // MinIO; nil disables attachment endpoints
	Entitlements EntitlementResolver // plan storage ceiling; nil ⇒ Free const
	Events       notify.EventBus     // realtime publish; nil ⇒ no SSE events
	Notifier     Notifier            // notification fan-out; nil ⇒ no fan-out
	Automation   automation.Evaluator  // automation rules; nil ⇒ no evaluation
	Logger       *slog.Logger
	Now          func() time.Time // injectable for tests; defaults to time.Now
}

// Service implements the task application logic.
type Service struct {
	tasks        project.TaskRepository
	subtasks     project.SubtaskRepository
	labels       project.LabelRepository
	comments     project.CommentRepository
	activity     project.ActivityRepository
	attachments  project.AttachmentRepository
	timeEntries  project.TimeEntryRepository
	links        project.TaskLinkRepository
	projects     project.ProjectRepository
	members      project.ProjectMemberRepository
	boards       project.BoardRepository
	columns      project.ColumnRepository
	store        project.ObjectStore
	entitlements EntitlementResolver
	events       notify.EventBus
	notifier     Notifier
	automation   automation.Evaluator
	logger       *slog.Logger
	now          func() time.Time
}

// New builds a Service from Deps.
func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		tasks: d.Tasks, subtasks: d.Subtasks, labels: d.Labels, comments: d.Comments,
		activity: d.Activity, attachments: d.Attachments, timeEntries: d.TimeEntries, links: d.Links, projects: d.Projects,
		members: d.Members, boards: d.Boards, columns: d.Columns, store: d.Store,
		entitlements: d.Entitlements, events: d.Events, notifier: d.Notifier,
		automation: d.Automation,
		logger: logger, now: now,
	}
}

// automate evaluates automation rules best-effort (nil ⇒ disabled). It never
// fails the write that triggered it.
func (s *Service) automate(ctx context.Context, e automation.Event) {
	if s.automation == nil {
		return
	}
	s.automation.Evaluate(ctx, e)
}

// publish emits a realtime event (best-effort; a publish failure is logged, not
// returned — realtime is not on the critical write path).
func (s *Service) publish(ctx context.Context, orgID, name, actorID string, data map[string]any) {
	if s.events == nil {
		return
	}
	if _, err := s.events.Publish(ctx, orgID, notify.NewEvent(name, actorID, data)); err != nil {
		s.logger.Warn("event publish failed", "event", name, "err", err)
	}
}

func newID() string { return uuidv7.New().String() }

// ---- Authorization (mirrors projectuc; kept local so the services stay
// independent) --------------------------------------------------------------

func (s *Service) effectiveRole(ctx context.Context, orgID string, p *project.Project, userID string, orgRole tenant.OrgRole) (project.ProjectRole, error) {
	if orgRole.AtLeast(tenant.RoleAdmin) {
		return project.RoleLead, nil
	}
	m, err := s.members.Get(ctx, orgID, p.ID, userID)
	if err == nil {
		return m.Role, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", err
	}
	if p.Visibility == project.VisibilityOrg && orgRole.AtLeast(tenant.RoleMember) {
		return project.RoleViewer, nil
	}
	return "", domain.ErrNotFound
}

// access requires at least min on the project. Below min → ErrForbidden.
func (s *Service) access(ctx context.Context, orgID, projectID, userID string, orgRole tenant.OrgRole, min project.ProjectRole) (project.ProjectRole, error) {
	p, err := s.projects.Get(ctx, orgID, projectID)
	if err != nil {
		return "", err
	}
	role, err := s.effectiveRole(ctx, orgID, p, userID, orgRole)
	if err != nil {
		return "", err
	}
	if !role.AtLeast(min) {
		return "", domain.ErrForbidden
	}
	return role, nil
}

// taskAccess loads a task and requires min on its project.
func (s *Service) taskAccess(ctx context.Context, orgID, taskID, userID string, orgRole tenant.OrgRole, min project.ProjectRole) (*project.Task, project.ProjectRole, error) {
	t, err := s.tasks.Get(ctx, orgID, taskID)
	if err != nil {
		return nil, "", err
	}
	role, err := s.access(ctx, orgID, t.ProjectID, userID, orgRole, min)
	if err != nil {
		return nil, "", err
	}
	return t, role, nil
}

// ---- Tasks ----------------------------------------------------------------

// CreateTaskInput is the payload for CreateTask.
type CreateTaskInput struct {
	ProjectID   string
	ColumnID    string
	Title       string
	Description string
	AssigneeID  *string
	Priority    project.Priority
	DueDate     *time.Time
}

// CreateTask creates a task at the end of its column (CONTRIBUTOR+, FR-TASK-001).
func (s *Service) CreateTask(ctx context.Context, orgID, userID string, in CreateTaskInput, orgRole tenant.OrgRole) (*project.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > project.MaxTitleLen {
		return nil, domain.ErrValidation
	}
	if in.Priority == "" {
		in.Priority = project.PriorityNone
	}
	if !in.Priority.Valid() {
		return nil, domain.ErrValidation
	}
	if _, err := s.access(ctx, orgID, in.ProjectID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	if err := s.columnInProject(ctx, orgID, in.ProjectID, in.ColumnID); err != nil {
		return nil, err
	}
	existing, err := s.tasks.ListByColumn(ctx, orgID, in.ColumnID)
	if err != nil {
		return nil, err
	}
	last := ""
	if n := len(existing); n > 0 {
		last = existing[n-1].Rank
	}
	t := &project.Task{
		ID: newID(), OrgID: orgID, ProjectID: in.ProjectID, ColumnID: in.ColumnID,
		Title: in.Title, Description: in.Description, AssigneeID: in.AssigneeID,
		Priority: in.Priority, DueDate: in.DueDate, Rank: rank.Append(last), CreatedBy: userID,
	}
	if err := s.tasks.Create(ctx, orgID, t); err != nil {
		return nil, err
	}
	s.publish(ctx, orgID, notify.EventTaskCreated, userID, map[string]any{
		"task_id": t.ID, "project_id": t.ProjectID, "column_id": t.ColumnID, "title": t.Title,
	})
	s.automate(ctx, automation.Event{
		OrgID: orgID, Trigger: automation.TriggerTaskCreated, TaskID: t.ID, ActorID: userID,
	})
	if t.AssigneeID != nil && s.notifier != nil {
		s.notifier.NotifyAssigned(ctx, orgID, userID, t.ID, *t.AssigneeID, t.Title)
	}
	return s.tasks.Get(ctx, orgID, t.ID)
}

// GetTask returns a task the caller may view.
func (s *Service) GetTask(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) (*project.Task, error) {
	t, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer)
	return t, err
}

// ListTasksByColumn returns a column's tasks (viewer).
func (s *Service) ListTasksByColumn(ctx context.Context, orgID, userID, projectID, columnID string, orgRole tenant.OrgRole) ([]project.Task, error) {
	if _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.columnInProject(ctx, orgID, projectID, columnID); err != nil {
		return nil, err
	}
	return s.tasks.ListByColumn(ctx, orgID, columnID)
}

// UpdateTaskInput carries the full new value of every editable field; each
// changed field is written to the activity log (FR-TASK-002).
type UpdateTaskInput struct {
	Title       string
	Description string
	AssigneeID  *string
	Priority    project.Priority
	DueDate     *time.Time
}

// UpdateTask edits a task's fields and records each change (CONTRIBUTOR+).
func (s *Service) UpdateTask(ctx context.Context, orgID, userID, taskID string, in UpdateTaskInput, orgRole tenant.OrgRole) (*project.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > project.MaxTitleLen {
		return nil, domain.ErrValidation
	}
	if in.Priority == "" {
		in.Priority = project.PriorityNone
	}
	if !in.Priority.Valid() {
		return nil, domain.ErrValidation
	}
	t, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor)
	if err != nil {
		return nil, err
	}

	changes := diffTask(t, in)
	t.Title = in.Title
	t.Description = in.Description
	t.AssigneeID = in.AssigneeID
	t.Priority = in.Priority
	t.DueDate = in.DueDate
	if err := s.tasks.Update(ctx, orgID, t); err != nil {
		return nil, err
	}
	fields := make([]string, 0, len(changes))
	assigneeChanged := false
	for _, c := range changes {
		a := &project.Activity{ID: newID(), TaskID: taskID, ActorID: userID, Field: c.field, OldValue: c.old, NewValue: c.new}
		if err := s.activity.Append(ctx, orgID, a); err != nil {
			s.logger.Warn("activity append failed", "task", taskID, "field", c.field, "err", err)
		}
		fields = append(fields, c.field)
		if c.field == "assignee" {
			assigneeChanged = true
		}
	}
	if len(changes) > 0 {
		s.publish(ctx, orgID, notify.EventTaskUpdated, userID, map[string]any{
			"task_id": taskID, "project_id": t.ProjectID, "fields": fields,
		})
	}
	if assigneeChanged && in.AssigneeID != nil && s.notifier != nil {
		s.notifier.NotifyAssigned(ctx, orgID, userID, taskID, *in.AssigneeID, t.Title)
	}
	if assigneeChanged {
		assignee := ""
		if in.AssigneeID != nil {
			assignee = *in.AssigneeID
		}
		s.automate(ctx, automation.Event{
			OrgID: orgID, Trigger: automation.TriggerTaskAssigned,
			TaskID: taskID, ActorID: userID, AssigneeID: assignee,
		})
	}
	return s.tasks.Get(ctx, orgID, taskID)
}

// ListActivity returns a task's change history (viewer, FR-TASK-002).
func (s *Service) ListActivity(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.Activity, error) {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.activity.ListByTask(ctx, orgID, taskID)
}

type fieldChange struct {
	field    string
	old, new *string
}

// diffTask returns one entry per changed editable field.
func diffTask(t *project.Task, in UpdateTaskInput) []fieldChange {
	var out []fieldChange
	if t.Title != in.Title {
		out = append(out, fieldChange{"title", strPtr(t.Title), strPtr(in.Title)})
	}
	if t.Description != in.Description {
		out = append(out, fieldChange{"description", strPtr(t.Description), strPtr(in.Description)})
	}
	if !eqPtr(t.AssigneeID, in.AssigneeID) {
		out = append(out, fieldChange{"assignee", t.AssigneeID, in.AssigneeID})
	}
	if t.Priority != in.Priority {
		out = append(out, fieldChange{"priority", strPtr(string(t.Priority)), strPtr(string(in.Priority))})
	}
	if !eqTimePtr(t.DueDate, in.DueDate) {
		out = append(out, fieldChange{"due_date", timePtr(t.DueDate), timePtr(in.DueDate)})
	}
	return out
}

// ---- Subtasks (FR-TASK-003) -----------------------------------------------

// AddSubtask appends a subtask (CONTRIBUTOR+).
func (s *Service) AddSubtask(ctx context.Context, orgID, userID, taskID, title string, orgRole tenant.OrgRole) (*project.Subtask, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, domain.ErrValidation
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	existing, err := s.subtasks.ListByTask(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}
	last := ""
	if n := len(existing); n > 0 {
		last = existing[n-1].Rank
	}
	st := &project.Subtask{ID: newID(), TaskID: taskID, Title: title, Rank: rank.Append(last)}
	if err := s.subtasks.Create(ctx, orgID, st); err != nil {
		return nil, err
	}
	return s.subtasks.Get(ctx, orgID, st.ID)
}

// ListSubtasks returns a task's subtasks (viewer).
func (s *Service) ListSubtasks(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.Subtask, error) {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.subtasks.ListByTask(ctx, orgID, taskID)
}

// UpdateSubtask edits a subtask's title/done (CONTRIBUTOR+).
func (s *Service) UpdateSubtask(ctx context.Context, orgID, userID, taskID, subtaskID, title string, done bool, orgRole tenant.OrgRole) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return domain.ErrValidation
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	if err := s.subtaskInTask(ctx, orgID, taskID, subtaskID); err != nil {
		return err
	}
	return s.subtasks.Update(ctx, orgID, subtaskID, title, done)
}

// DeleteSubtask removes a subtask (CONTRIBUTOR+).
func (s *Service) DeleteSubtask(ctx context.Context, orgID, userID, taskID, subtaskID string, orgRole tenant.OrgRole) error {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	if err := s.subtaskInTask(ctx, orgID, taskID, subtaskID); err != nil {
		return err
	}
	return s.subtasks.Delete(ctx, orgID, subtaskID)
}

// ---- Labels (FR-TASK-004) -------------------------------------------------
// Labels are org-scoped; the org-role gate (MEMBER+ write:labels) is enforced by
// the caller's middleware.

// CreateLabel creates an org label.
func (s *Service) CreateLabel(ctx context.Context, orgID, name, color string) (*project.Label, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > project.MaxLabelLen {
		return nil, domain.ErrValidation
	}
	l := &project.Label{ID: newID(), OrgID: orgID, Name: name, Color: color}
	if err := s.labels.Create(ctx, orgID, l); err != nil {
		return nil, err // ErrConflict on duplicate name
	}
	return s.labels.Get(ctx, orgID, l.ID)
}

// ListLabels returns the org's labels.
func (s *Service) ListLabels(ctx context.Context, orgID string) ([]project.Label, error) {
	return s.labels.List(ctx, orgID)
}

// UpdateLabel edits a label's name/color.
func (s *Service) UpdateLabel(ctx context.Context, orgID, labelID, name, color string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > project.MaxLabelLen {
		return domain.ErrValidation
	}
	return s.labels.Update(ctx, orgID, labelID, name, color)
}

// DeleteLabel removes a label; the DB cascade detaches it from every task.
func (s *Service) DeleteLabel(ctx context.Context, orgID, labelID string) error {
	return s.labels.Delete(ctx, orgID, labelID)
}

// AttachLabel links a label to a task (CONTRIBUTOR+ on the task's project).
func (s *Service) AttachLabel(ctx context.Context, orgID, userID, taskID, labelID string, orgRole tenant.OrgRole) error {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	if _, err := s.labels.Get(ctx, orgID, labelID); err != nil {
		return err
	}
	return s.labels.Attach(ctx, orgID, taskID, labelID)
}

// DetachLabel unlinks a label from a task (CONTRIBUTOR+).
func (s *Service) DetachLabel(ctx context.Context, orgID, userID, taskID, labelID string, orgRole tenant.OrgRole) error {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	return s.labels.Detach(ctx, orgID, taskID, labelID)
}

// ListTaskLabels returns a task's labels (viewer).
func (s *Service) ListTaskLabels(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.Label, error) {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.labels.ListForTask(ctx, orgID, taskID)
}

// ---- Comments (FR-TASK-005) -----------------------------------------------

// AddComment posts a comment (CONTRIBUTOR+).
func (s *Service) AddComment(ctx context.Context, orgID, userID, taskID, body string, orgRole tenant.OrgRole) (*project.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, domain.ErrValidation
	}
	t, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor)
	if err != nil {
		return nil, err
	}
	c := &project.Comment{ID: newID(), TaskID: taskID, AuthorID: userID, Body: body}
	if err := s.comments.Create(ctx, orgID, c); err != nil {
		return nil, err
	}
	s.publish(ctx, orgID, notify.EventCommentCreated, userID, map[string]any{
		"task_id": taskID, "comment_id": c.ID, "project_id": t.ProjectID,
	})
	// Fan out @mentions (project members) + comment targets (task creator +
	// assignee) to the notification center + email (FR-TASK-005 → FR-NTF-002).
	if s.notifier != nil {
		targets := []string{t.CreatedBy}
		if t.AssigneeID != nil {
			targets = append(targets, *t.AssigneeID)
		}
		s.notifier.FanOutComment(ctx, orgID, userID, t.ProjectID, taskID, c.ID, body, targets)
	}
	return s.comments.Get(ctx, orgID, c.ID)
}

// ListComments returns a task's comments including soft-deleted placeholders
// (viewer).
func (s *Service) ListComments(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.Comment, error) {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.comments.ListByTask(ctx, orgID, taskID)
}

// EditComment rewrites a comment. Only the author, within the 15-minute window,
// may edit (FR-TASK-005). Not author → ErrForbidden; window passed → ErrConflict.
func (s *Service) EditComment(ctx context.Context, orgID, userID, taskID, commentID, body string, orgRole tenant.OrgRole) (*project.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, domain.ErrValidation
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	c, err := s.comments.Get(ctx, orgID, commentID)
	if err != nil {
		return nil, err
	}
	if c.TaskID != taskID || c.Deleted() {
		return nil, domain.ErrNotFound
	}
	if c.AuthorID != userID {
		return nil, domain.ErrForbidden
	}
	if !c.Editable(s.now().UTC()) {
		return nil, domain.ErrConflict // edit window elapsed
	}
	if err := s.comments.Update(ctx, orgID, commentID, body); err != nil {
		return nil, err
	}
	return s.comments.Get(ctx, orgID, commentID)
}

// DeleteComment soft-deletes a comment. The author or a project LEAD may delete
// (FR-TASK-005).
func (s *Service) DeleteComment(ctx context.Context, orgID, userID, taskID, commentID string, orgRole tenant.OrgRole) error {
	_, role, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer)
	if err != nil {
		return err
	}
	c, err := s.comments.Get(ctx, orgID, commentID)
	if err != nil {
		return err
	}
	if c.TaskID != taskID || c.Deleted() {
		return domain.ErrNotFound
	}
	if c.AuthorID != userID && !role.AtLeast(project.RoleLead) {
		return domain.ErrForbidden
	}
	return s.comments.SoftDelete(ctx, orgID, commentID)
}

// ---- Helpers --------------------------------------------------------------

// columnInProject verifies a column belongs to the project's board.
func (s *Service) columnInProject(ctx context.Context, orgID, projectID, columnID string) error {
	col, err := s.columns.Get(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	board, err := s.boards.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return err
	}
	if col.BoardID != board.ID {
		return domain.ErrValidation
	}
	return nil
}

// subtaskInTask verifies a subtask belongs to the task.
func (s *Service) subtaskInTask(ctx context.Context, orgID, taskID, subtaskID string) error {
	st, err := s.subtasks.Get(ctx, orgID, subtaskID)
	if err != nil {
		return err
	}
	if st.TaskID != taskID {
		return domain.ErrNotFound
	}
	return nil
}

func strPtr(s string) *string { return &s }

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func timePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
