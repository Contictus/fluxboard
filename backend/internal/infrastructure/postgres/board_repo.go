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

// BoardRepo is the Postgres-backed project.BoardRepository ([T], RLS).
type BoardRepo struct{ tp *TenantPool }

// NewBoardRepo builds a BoardRepo over the tenant pool.
func NewBoardRepo(tp *TenantPool) *BoardRepo { return &BoardRepo{tp: tp} }

var _ project.BoardRepository = (*BoardRepo)(nil)

func (r *BoardRepo) Create(ctx context.Context, orgID string, b *project.Board) error {
	id, err := parseUUID(b.ID)
	if err != nil {
		return fmt.Errorf("board create: id: %w", err)
	}
	pid, err := parseUUID(b.ProjectID)
	if err != nil {
		return fmt.Errorf("board create: project: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateBoard(ctx, gen.CreateBoardParams{ID: id, OrgID: oid, ProjectID: pid, Name: b.Name})
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *BoardRepo) GetByProject(ctx context.Context, orgID, projectID string) (*project.Board, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("board get: %w", err)
	}
	var out *project.Board
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetBoardByProject(ctx, gen.GetBoardByProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = &project.Board{
			ID: row.ID.String(), ProjectID: row.ProjectID.String(), Name: row.Name,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ColumnRepo is the Postgres-backed project.ColumnRepository ([T], RLS).
type ColumnRepo struct{ tp *TenantPool }

// NewColumnRepo builds a ColumnRepo over the tenant pool.
func NewColumnRepo(tp *TenantPool) *ColumnRepo { return &ColumnRepo{tp: tp} }

var _ project.ColumnRepository = (*ColumnRepo)(nil)

func mapColumn(row gen.BoardColumn) project.Column {
	return project.Column{
		ID: row.ID.String(), BoardID: row.BoardID.String(), Name: row.Name,
		Rank: row.Rank, WIPLimit: intPtr(row.WipLimit),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (r *ColumnRepo) Create(ctx context.Context, orgID string, c *project.Column) error {
	id, err := parseUUID(c.ID)
	if err != nil {
		return fmt.Errorf("column create: id: %w", err)
	}
	bid, err := parseUUID(c.BoardID)
	if err != nil {
		return fmt.Errorf("column create: board: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateColumn(ctx, gen.CreateColumnParams{
			ID: id, OrgID: oid, BoardID: bid, Name: c.Name, Rank: c.Rank, WipLimit: int32Ptr(c.WIPLimit),
		})
	})
}

func (r *ColumnRepo) Get(ctx context.Context, orgID, id string) (*project.Column, error) {
	cid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("column get: %w", err)
	}
	var out *project.Column
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetColumn(ctx, gen.GetColumnParams{OrgID: oid, ID: cid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		c := mapColumn(row)
		out = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ColumnRepo) ListByBoard(ctx context.Context, orgID, boardID string) ([]project.Column, error) {
	bid, err := parseUUID(boardID)
	if err != nil {
		return nil, fmt.Errorf("column list: %w", err)
	}
	var out []project.Column
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListColumnsByBoard(ctx, gen.ListColumnsByBoardParams{OrgID: oid, BoardID: bid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapColumn(row))
		}
		return nil
	})
	return out, err
}

func (r *ColumnRepo) Update(ctx context.Context, orgID, id, name string, wipLimit *int) error {
	cid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("column update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateColumn(ctx, gen.UpdateColumnParams{Name: name, WipLimit: int32Ptr(wipLimit), OrgID: oid, ID: cid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *ColumnRepo) SetRank(ctx context.Context, orgID, id, rank string) error {
	cid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("column rank: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.SetColumnRank(ctx, gen.SetColumnRankParams{Rank: rank, OrgID: oid, ID: cid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *ColumnRepo) Delete(ctx context.Context, orgID, id string) error {
	cid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("column delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteColumn(ctx, gen.DeleteColumnParams{OrgID: oid, ID: cid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *ColumnRepo) CountTasks(ctx context.Context, orgID, columnID string) (int, error) {
	cid, err := parseUUID(columnID)
	if err != nil {
		return 0, fmt.Errorf("column count: %w", err)
	}
	var n int
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		c, err := q.CountColumnTasks(ctx, gen.CountColumnTasksParams{OrgID: oid, ColumnID: cid})
		n = int(c)
		return err
	})
	return n, err
}
