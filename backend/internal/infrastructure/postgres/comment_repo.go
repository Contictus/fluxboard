package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// CommentRepo is the Postgres-backed project.CommentRepository ([T], RLS).
type CommentRepo struct{ tp *TenantPool }

// NewCommentRepo builds a CommentRepo over the tenant pool.
func NewCommentRepo(tp *TenantPool) *CommentRepo { return &CommentRepo{tp: tp} }

var _ project.CommentRepository = (*CommentRepo)(nil)

func mapComment(row gen.Comment) project.Comment {
	return project.Comment{
		ID: row.ID.String(), TaskID: row.TaskID.String(), AuthorID: row.AuthorID.String(),
		Body: row.Body, Edited: row.Edited, DeletedAt: tsPtr(row.DeletedAt),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (r *CommentRepo) Create(ctx context.Context, orgID string, c *project.Comment) error {
	id, err := parseUUID(c.ID)
	if err != nil {
		return fmt.Errorf("comment create: id: %w", err)
	}
	tid, err := parseUUID(c.TaskID)
	if err != nil {
		return fmt.Errorf("comment create: task: %w", err)
	}
	author, err := parseUUID(c.AuthorID)
	if err != nil {
		return fmt.Errorf("comment create: author: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateComment(ctx, gen.CreateCommentParams{ID: id, OrgID: oid, TaskID: tid, AuthorID: author, Body: c.Body})
	})
	if isForeignKey(err) {
		return domain.ErrNotFound
	}
	return err
}

func (r *CommentRepo) Get(ctx context.Context, orgID, id string) (*project.Comment, error) {
	cid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("comment get: %w", err)
	}
	var out *project.Comment
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetComment(ctx, gen.GetCommentParams{OrgID: oid, ID: cid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		c := mapComment(row)
		out = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommentRepo) ListByTask(ctx context.Context, orgID, taskID string) ([]project.Comment, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("comment list: %w", err)
	}
	var out []project.Comment
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListCommentsByTask(ctx, gen.ListCommentsByTaskParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapComment(row))
		}
		return nil
	})
	return out, err
}

// CountForProject returns per-task live comment counts in one query.
func (r *CommentRepo) CountForProject(ctx context.Context, orgID, projectID string) (map[string]int, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("comment count for project: %w", err)
	}
	out := make(map[string]int)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.CountCommentsForProject(ctx, gen.CountCommentsForProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out[row.TaskID.String()] = int(row.Total)
		}
		return nil
	})
	return out, err
}

func (r *CommentRepo) Update(ctx context.Context, orgID, id, body string) error {
	cid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("comment update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateComment(ctx, gen.UpdateCommentParams{Body: body, OrgID: oid, ID: cid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *CommentRepo) SoftDelete(ctx context.Context, orgID, id string) error {
	cid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("comment delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.SoftDeleteComment(ctx, gen.SoftDeleteCommentParams{OrgID: oid, ID: cid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
