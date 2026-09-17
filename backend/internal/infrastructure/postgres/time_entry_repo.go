package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// TimeEntryRepo is the Postgres-backed project.TimeEntryRepository ([T], RLS).
type TimeEntryRepo struct{ tp *TenantPool }

// NewTimeEntryRepo builds a TimeEntryRepo over the tenant pool.
func NewTimeEntryRepo(tp *TenantPool) *TimeEntryRepo { return &TimeEntryRepo{tp: tp} }

var _ project.TimeEntryRepository = (*TimeEntryRepo)(nil)

func mapTimeEntry(row gen.TimeEntry) *project.TimeEntry {
	e := &project.TimeEntry{
		ID: row.ID.String(), OrgID: row.OrgID.String(), TaskID: row.TaskID.String(),
		UserID: row.UserID.String(), StartedAt: row.StartedAt, Note: row.Note,
		CreatedAt: row.CreatedAt,
	}
	if row.EndedAt.Valid {
		ended := row.EndedAt.Time
		e.EndedAt = &ended
	}
	return e
}

func (r *TimeEntryRepo) Create(ctx context.Context, orgID string, e *project.TimeEntry) error {
	id, err := parseUUID(e.ID)
	if err != nil {
		return fmt.Errorf("time entry create: %w", err)
	}
	tid, err := parseUUID(e.TaskID)
	if err != nil {
		return fmt.Errorf("time entry create: task: %w", err)
	}
	uid, err := parseUUID(e.UserID)
	if err != nil {
		return fmt.Errorf("time entry create: user: %w", err)
	}
	var ended pgtype.Timestamptz
	if e.EndedAt != nil {
		ended = pgtype.Timestamptz{Time: *e.EndedAt, Valid: true}
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateTimeEntry(ctx, gen.CreateTimeEntryParams{
			ID: id, OrgID: oid, TaskID: tid, UserID: uid,
			StartedAt: e.StartedAt, EndedAt: ended, Note: e.Note,
		})
	})
}

func (r *TimeEntryRepo) Get(ctx context.Context, orgID, id string) (*project.TimeEntry, error) {
	eid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("time entry get: %w", err)
	}
	var out *project.TimeEntry
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetTimeEntry(ctx, gen.GetTimeEntryParams{OrgID: oid, ID: eid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = mapTimeEntry(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r *TimeEntryRepo) list(ctx context.Context, orgID string, fn func(q *gen.Queries) ([]gen.TimeEntry, error)) ([]project.TimeEntry, error) {
	out := make([]project.TimeEntry, 0)
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		rows, err := fn(q)
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, *mapTimeEntry(row))
		}
		return nil
	})
	return out, err
}

// ListByTask returns a task's entries newest-first.
func (r *TimeEntryRepo) ListByTask(ctx context.Context, orgID, taskID string) ([]project.TimeEntry, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("time entry list: %w", err)
	}
	return r.list(ctx, orgID, func(q *gen.Queries) ([]gen.TimeEntry, error) {
		oid, _ := parseUUID(orgID)
		return q.ListTimeEntriesByTask(ctx, gen.ListTimeEntriesByTaskParams{OrgID: oid, TaskID: tid})
	})
}

// ListRunning returns the caller's running timers newest-first.
func (r *TimeEntryRepo) ListRunning(ctx context.Context, orgID, userID string) ([]project.TimeEntry, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("time entry running: %w", err)
	}
	return r.list(ctx, orgID, func(q *gen.Queries) ([]gen.TimeEntry, error) {
		oid, _ := parseUUID(orgID)
		return q.ListRunningTimeEntries(ctx, gen.ListRunningTimeEntriesParams{OrgID: oid, UserID: uid})
	})
}

// Stop stamps ended_at. ErrNotFound if absent or already stopped.
func (r *TimeEntryRepo) Stop(ctx context.Context, orgID, id string, endedAt time.Time) error {
	eid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("time entry stop: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.StopTimeEntry(ctx, gen.StopTimeEntryParams{
			EndedAt: pgtype.Timestamptz{Time: endedAt, Valid: true}, OrgID: oid, ID: eid,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// Delete removes an entry. ErrNotFound if absent.
func (r *TimeEntryRepo) Delete(ctx context.Context, orgID, id string) error {
	eid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("time entry delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteTimeEntry(ctx, gen.DeleteTimeEntryParams{OrgID: oid, ID: eid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
