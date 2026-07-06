package taskuc

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/rank"
)

// ---- Trash: soft-delete / restore (FR-TASK-009) ---------------------------

// TrashTask soft-deletes a task into the Trash (CONTRIBUTOR+). It keeps its
// board slot for 30 days; the purge job hard-deletes it after that.
func (s *Service) TrashTask(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) error {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	return s.tasks.SoftDelete(ctx, orgID, taskID, s.now().UTC())
}

// RestoreTask returns a trashed task to its board (CONTRIBUTOR+). ErrConflict if
// its original (column, rank) slot was reclaimed while it was trashed.
func (s *Service) RestoreTask(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) (*project.Task, error) {
	t, err := s.tasks.GetTrashed(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}
	if _, err := s.access(ctx, orgID, t.ProjectID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	if err := s.tasks.Restore(ctx, orgID, taskID); err != nil {
		return nil, err
	}
	return s.tasks.Get(ctx, orgID, taskID)
}

// ListTrash returns a project's trashed tasks (VIEWER+).
func (s *Service) ListTrash(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) ([]project.Task, error) {
	if _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.tasks.ListTrashed(ctx, orgID, projectID)
}

// PurgeExpiredTrash hard-deletes tasks trashed longer than the retention window.
// Maintenance entrypoint for the nightly job (no user context; per-tenant).
func (s *Service) PurgeExpiredTrash(ctx context.Context, orgID string) (int, error) {
	cutoff := s.now().UTC().Add(-project.TrashRetention)
	return s.tasks.PurgeExpired(ctx, orgID, cutoff)
}

// ---- Search (FR-TASK-007) -------------------------------------------------

// Search runs a paginated full-text search over the caller's visible tasks.
// Visibility (org vs. private-membership) is enforced in the query; org ADMIN+
// see every project.
func (s *Service) Search(ctx context.Context, orgID, userID string, f project.SearchFilter, orgRole tenant.OrgRole) (*project.SearchResult, error) {
	f.Query = strings.TrimSpace(f.Query)
	if f.Query == "" {
		return nil, domain.ErrValidation
	}
	if f.Priority != "" && !project.Priority(f.Priority).Valid() {
		return nil, domain.ErrValidation
	}
	if f.Limit <= 0 {
		f.Limit = project.DefaultSearchLimit
	}
	if f.Limit > project.MaxSearchLimit {
		f.Limit = project.MaxSearchLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.UserID = userID
	f.SeeAll = orgRole.AtLeast(tenant.RoleAdmin)
	return s.tasks.Search(ctx, orgID, f)
}

// ---- Bulk actions (FR-TASK-008) -------------------------------------------

// bulkAuthorize loads every task, requires CONTRIBUTOR+ on each distinct project,
// and returns the loaded tasks. ErrNotFound if any id is missing/trashed.
func (s *Service) bulkAuthorize(ctx context.Context, orgID, userID string, taskIDs []string, orgRole tenant.OrgRole) ([]*project.Task, error) {
	if len(taskIDs) == 0 || len(taskIDs) > project.MaxBulkTasks {
		return nil, domain.ErrValidation
	}
	seen := make(map[string]bool, len(taskIDs))
	checked := make(map[string]bool)
	tasks := make([]*project.Task, 0, len(taskIDs))
	for _, id := range taskIDs {
		if seen[id] {
			return nil, domain.ErrValidation // duplicate id in the set
		}
		seen[id] = true
		t, err := s.tasks.Get(ctx, orgID, id)
		if err != nil {
			return nil, err
		}
		if !checked[t.ProjectID] {
			if _, err := s.access(ctx, orgID, t.ProjectID, userID, orgRole, project.RoleContributor); err != nil {
				return nil, err
			}
			checked[t.ProjectID] = true
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// BulkAssign sets (or clears, when assigneeID is nil) the assignee on every task
// in one transaction (CONTRIBUTOR+ on each task's project).
func (s *Service) BulkAssign(ctx context.Context, orgID, userID string, taskIDs []string, assigneeID *string, orgRole tenant.OrgRole) error {
	if _, err := s.bulkAuthorize(ctx, orgID, userID, taskIDs, orgRole); err != nil {
		return err
	}
	return s.tasks.BulkAssign(ctx, orgID, taskIDs, assigneeID)
}

// BulkAddLabel attaches a label to every task (CONTRIBUTOR+). The label must
// exist in the org.
func (s *Service) BulkAddLabel(ctx context.Context, orgID, userID string, taskIDs []string, labelID string, orgRole tenant.OrgRole) error {
	if _, err := s.bulkAuthorize(ctx, orgID, userID, taskIDs, orgRole); err != nil {
		return err
	}
	if _, err := s.labels.Get(ctx, orgID, labelID); err != nil {
		return err
	}
	for _, id := range taskIDs {
		if err := s.labels.Attach(ctx, orgID, id, labelID); err != nil {
			return err
		}
	}
	return nil
}

// BulkMove relocates every task into columnID (CONTRIBUTOR+). All tasks must
// share the target column's project; each is appended a distinct rank in one
// transaction. ErrConflict on a rank clash (concurrent move).
func (s *Service) BulkMove(ctx context.Context, orgID, userID string, taskIDs []string, columnID string, orgRole tenant.OrgRole) error {
	tasks, err := s.bulkAuthorize(ctx, orgID, userID, taskIDs, orgRole)
	if err != nil {
		return err
	}
	projectID := tasks[0].ProjectID
	for _, t := range tasks {
		if t.ProjectID != projectID {
			return domain.ErrValidation // cross-project bulk move is rejected
		}
	}
	if err := s.columnInProject(ctx, orgID, projectID, columnID); err != nil {
		return err
	}
	existing, err := s.tasks.ListByColumn(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	last := ""
	if n := len(existing); n > 0 {
		last = existing[n-1].Rank
	}
	ranks := make([]string, len(taskIDs))
	for i := range taskIDs {
		last = rank.Append(last)
		ranks[i] = last
	}
	return s.tasks.BulkMove(ctx, orgID, columnID, taskIDs, ranks)
}

// ---- Attachments (FR-TASK-006) --------------------------------------------

// attachmentsEnabled reports whether object storage is wired.
func (s *Service) attachmentsEnabled() bool { return s.store != nil }

// RequestUpload validates an upload and returns the pending row plus a presigned
// PUT URL the client uploads to directly (CONTRIBUTOR+). The API never proxies
// bytes. ErrConflict when the org storage quota would be exceeded.
func (s *Service) RequestUpload(ctx context.Context, orgID, userID, taskID, filename, contentType string, size int64, orgRole tenant.OrgRole) (*project.Attachment, string, error) {
	if !s.attachmentsEnabled() {
		return nil, "", domain.ErrForbidden // attachments disabled (no MinIO)
	}
	filename = strings.TrimSpace(filename)
	if filename == "" || size <= 0 || size > project.MaxAttachmentBytes {
		return nil, "", domain.ErrValidation
	}
	if !project.AllowedMIME(contentType) {
		return nil, "", domain.ErrValidation
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, "", err
	}
	used, err := s.attachments.SumOrgBytes(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	if used+size > project.OrgStorageQuotaBytes {
		return nil, "", domain.ErrConflict // storage quota exceeded
	}
	id := newID()
	key := objectKey(orgID, taskID, id, filename)
	a := &project.Attachment{
		ID: id, OrgID: orgID, TaskID: taskID, UploaderID: userID, ObjectKey: key,
		Filename: filename, ContentType: contentType, SizeBytes: size,
		Status: project.AttachmentPending,
	}
	if err := s.attachments.Create(ctx, orgID, a); err != nil {
		return nil, "", err
	}
	url, err := s.store.PresignPut(ctx, key, contentType, project.UploadURLTTL)
	if err != nil {
		return nil, "", err
	}
	return a, url, nil
}

// ConfirmUpload verifies the object exists in storage (HEAD) and commits the row
// (CONTRIBUTOR+). ErrValidation if the client never completed the PUT or the
// stored size is out of bounds.
func (s *Service) ConfirmUpload(ctx context.Context, orgID, userID, attachmentID string, orgRole tenant.OrgRole) (*project.Attachment, error) {
	if !s.attachmentsEnabled() {
		return nil, domain.ErrForbidden
	}
	a, err := s.attachments.Get(ctx, orgID, attachmentID)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.taskAccess(ctx, orgID, a.TaskID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	size, err := s.store.Stat(ctx, a.ObjectKey)
	if err != nil {
		return nil, domain.ErrValidation // object not uploaded yet
	}
	if size <= 0 || size > project.MaxAttachmentBytes {
		_ = s.store.Remove(ctx, a.ObjectKey) // reject oversized upload
		return nil, domain.ErrValidation
	}
	if err := s.attachments.Commit(ctx, orgID, attachmentID, size, s.now().UTC()); err != nil {
		return nil, err
	}
	return s.attachments.Get(ctx, orgID, attachmentID)
}

// ListAttachments returns a task's committed attachments (VIEWER+).
func (s *Service) ListAttachments(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.Attachment, error) {
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.attachments.ListByTask(ctx, orgID, taskID)
}

// DownloadURL returns a short-lived presigned GET for a committed attachment
// (VIEWER+).
func (s *Service) DownloadURL(ctx context.Context, orgID, userID, attachmentID string, orgRole tenant.OrgRole) (string, error) {
	if !s.attachmentsEnabled() {
		return "", domain.ErrForbidden
	}
	a, err := s.attachments.Get(ctx, orgID, attachmentID)
	if err != nil {
		return "", err
	}
	if _, _, err := s.taskAccess(ctx, orgID, a.TaskID, userID, orgRole, project.RoleViewer); err != nil {
		return "", err
	}
	if a.Status != project.AttachmentCommitted {
		return "", domain.ErrNotFound
	}
	return s.store.PresignGet(ctx, a.ObjectKey, a.Filename, project.DownloadURLTTL)
}

// DeleteAttachment removes an attachment's object and row (CONTRIBUTOR+).
func (s *Service) DeleteAttachment(ctx context.Context, orgID, userID, attachmentID string, orgRole tenant.OrgRole) error {
	if !s.attachmentsEnabled() {
		return domain.ErrForbidden
	}
	a, err := s.attachments.Get(ctx, orgID, attachmentID)
	if err != nil {
		return err
	}
	if _, _, err := s.taskAccess(ctx, orgID, a.TaskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	if err := s.store.Remove(ctx, a.ObjectKey); err != nil {
		return err
	}
	return s.attachments.Delete(ctx, orgID, attachmentID)
}

// GCOrphanAttachments removes pending attachments older than the GC age and their
// objects. Maintenance entrypoint for the nightly job (per-tenant). Returns the
// number of orphans reclaimed.
func (s *Service) GCOrphanAttachments(ctx context.Context, orgID string) (int, error) {
	if !s.attachmentsEnabled() {
		return 0, nil
	}
	cutoff := s.now().UTC().Add(-project.OrphanGCAge)
	orphans, err := s.attachments.ListOrphans(ctx, orgID, cutoff)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range orphans {
		o := &orphans[i]
		if err := s.store.Remove(ctx, o.ObjectKey); err != nil {
			s.logger.Warn("orphan gc: remove object failed", "key", o.ObjectKey, "err", err)
			continue
		}
		if err := s.attachments.Delete(ctx, orgID, o.ID); err != nil {
			s.logger.Warn("orphan gc: delete row failed", "id", o.ID, "err", err)
			continue
		}
		n++
	}
	return n, nil
}

// objectKey builds a stable, collision-free MinIO key for an attachment.
func objectKey(orgID, taskID, attachmentID, filename string) string {
	return fmt.Sprintf("orgs/%s/tasks/%s/%s/%s", orgID, taskID, attachmentID, path.Base(filename))
}
