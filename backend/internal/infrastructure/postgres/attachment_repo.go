package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// AttachmentRepo is the Postgres-backed project.AttachmentRepository ([T], RLS).
type AttachmentRepo struct{ tp *TenantPool }

// NewAttachmentRepo builds an AttachmentRepo over the tenant pool.
func NewAttachmentRepo(tp *TenantPool) *AttachmentRepo { return &AttachmentRepo{tp: tp} }

var _ project.AttachmentRepository = (*AttachmentRepo)(nil)

func mapAttachment(row gen.Attachment) project.Attachment {
	return project.Attachment{
		ID: row.ID.String(), OrgID: row.OrgID.String(), TaskID: row.TaskID.String(),
		UploaderID: row.UploaderID.String(), ObjectKey: row.ObjectKey, Filename: row.Filename,
		ContentType: row.ContentType, SizeBytes: row.SizeBytes,
		Status: project.AttachmentStatus(row.Status), CreatedAt: row.CreatedAt,
		ConfirmedAt: tsPtr(row.ConfirmedAt),
	}
}

func (r *AttachmentRepo) Create(ctx context.Context, orgID string, a *project.Attachment) error {
	id, err := parseUUID(a.ID)
	if err != nil {
		return fmt.Errorf("attachment create: id: %w", err)
	}
	tid, err := parseUUID(a.TaskID)
	if err != nil {
		return fmt.Errorf("attachment create: task: %w", err)
	}
	uploader, err := parseUUID(a.UploaderID)
	if err != nil {
		return fmt.Errorf("attachment create: uploader: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateAttachment(ctx, gen.CreateAttachmentParams{
			ID: id, OrgID: oid, TaskID: tid, UploaderID: uploader,
			ObjectKey: a.ObjectKey, Filename: a.Filename, ContentType: a.ContentType,
			SizeBytes: a.SizeBytes,
		})
	})
	if isForeignKey(err) {
		return domain.ErrNotFound // task gone
	}
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *AttachmentRepo) Get(ctx context.Context, orgID, id string) (*project.Attachment, error) {
	aid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("attachment get: %w", err)
	}
	var out *project.Attachment
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetAttachment(ctx, gen.GetAttachmentParams{OrgID: oid, ID: aid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		a := mapAttachment(row)
		out = &a
		return nil
	})
	return out, err
}

func (r *AttachmentRepo) Commit(ctx context.Context, orgID, id string, sizeBytes int64, confirmedAt time.Time) error {
	aid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("attachment commit: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.CommitAttachment(ctx, gen.CommitAttachmentParams{
			SizeBytes: sizeBytes, ConfirmedAt: tsVal(confirmedAt), OrgID: oid, ID: aid,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound // absent or already committed
		}
		return nil
	})
}

func (r *AttachmentRepo) ListByTask(ctx context.Context, orgID, taskID string) ([]project.Attachment, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("attachment list: %w", err)
	}
	var out []project.Attachment
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListAttachmentsByTask(ctx, gen.ListAttachmentsByTaskParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapAttachment(row))
		}
		return nil
	})
	return out, err
}

func (r *AttachmentRepo) Delete(ctx context.Context, orgID, id string) error {
	aid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("attachment delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		_, err := q.DeleteAttachment(ctx, gen.DeleteAttachmentParams{OrgID: oid, ID: aid})
		return err
	})
}

func (r *AttachmentRepo) SumOrgBytes(ctx context.Context, orgID string) (int64, error) {
	var sum int64
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		var err error
		sum, err = q.SumOrgAttachmentBytes(ctx, oid)
		return err
	})
	return sum, err
}

func (r *AttachmentRepo) ListOrphans(ctx context.Context, orgID string, cutoff time.Time) ([]project.Attachment, error) {
	var out []project.Attachment
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListOrphanAttachments(ctx, gen.ListOrphanAttachmentsParams{OrgID: oid, Cutoff: cutoff})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapAttachment(row))
		}
		return nil
	})
	return out, err
}
