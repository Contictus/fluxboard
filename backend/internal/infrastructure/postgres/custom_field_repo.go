package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// CustomFieldRepo is the Postgres-backed project.CustomFieldRepository ([T], RLS).
type CustomFieldRepo struct{ tp *TenantPool }

// NewCustomFieldRepo builds a CustomFieldRepo over the tenant pool.
func NewCustomFieldRepo(tp *TenantPool) *CustomFieldRepo { return &CustomFieldRepo{tp: tp} }

var _ project.CustomFieldRepository = (*CustomFieldRepo)(nil)

func mapCustomField(row gen.CustomField) (*project.CustomField, error) {
	f := &project.CustomField{
		ID: row.ID.String(), OrgID: row.OrgID.String(), ProjectID: row.ProjectID.String(),
		Name: row.Name, Type: row.Type, Position: int(row.Position),
		CreatedBy: row.CreatedBy.String(), CreatedAt: row.CreatedAt,
	}
	if len(row.Options) > 0 {
		if err := json.Unmarshal(row.Options, &f.Options); err != nil {
			return nil, fmt.Errorf("custom field options: %w", err)
		}
	}
	return f, nil
}

func numberToNumeric(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f == nil {
		return n
	}
	_ = n.Scan(strconv.FormatFloat(*f, 'f', -1, 64))
	return n
}

func numericToFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func (r *CustomFieldRepo) Create(ctx context.Context, orgID string, f *project.CustomField) error {
	id, err := parseUUID(f.ID)
	if err != nil {
		return fmt.Errorf("custom field create: %w", err)
	}
	pid, err := parseUUID(f.ProjectID)
	if err != nil {
		return fmt.Errorf("custom field create: project: %w", err)
	}
	by, err := parseUUID(f.CreatedBy)
	if err != nil {
		return fmt.Errorf("custom field create: by: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateCustomField(ctx, gen.CreateCustomFieldParams{
			ID: id, OrgID: oid, ProjectID: pid, Name: f.Name, Type: f.Type,
			Options: marshalConfig(f.Options), Position: int32(f.Position), CreatedBy: by,
		})
	})
}

func (r *CustomFieldRepo) Get(ctx context.Context, orgID, id string) (*project.CustomField, error) {
	fid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("custom field get: %w", err)
	}
	var out *project.CustomField
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetCustomField(ctx, gen.GetCustomFieldParams{OrgID: oid, ID: fid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out, err = mapCustomField(row)
		return err
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r *CustomFieldRepo) ListByProject(ctx context.Context, orgID, projectID string) ([]project.CustomField, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("custom field list: %w", err)
	}
	out := make([]project.CustomField, 0)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListCustomFieldsByProject(ctx, gen.ListCustomFieldsByProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			mapped, err := mapCustomField(row)
			if err != nil {
				return err
			}
			out = append(out, *mapped)
		}
		return nil
	})
	return out, err
}

func (r *CustomFieldRepo) Update(ctx context.Context, orgID string, f *project.CustomField) error {
	fid, err := parseUUID(f.ID)
	if err != nil {
		return fmt.Errorf("custom field update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateCustomField(ctx, gen.UpdateCustomFieldParams{
			OrgID: oid, ID: fid, Name: f.Name,
			Options: marshalConfig(f.Options), Position: int32(f.Position),
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

func (r *CustomFieldRepo) Delete(ctx context.Context, orgID, id string) error {
	fid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("custom field delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteCustomField(ctx, gen.DeleteCustomFieldParams{OrgID: oid, ID: fid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *CustomFieldRepo) SetValue(ctx context.Context, orgID, taskID, fieldID string, v project.CustomValue) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("custom value set: task: %w", err)
	}
	fid, err := parseUUID(fieldID)
	if err != nil {
		return fmt.Errorf("custom value set: field: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.UpsertTaskCustomValue(ctx, gen.UpsertTaskCustomValueParams{
			TaskID: tid, FieldID: fid, OrgID: oid,
			ValueText: v.Text, ValueNumber: numberToNumeric(v.Number), ValueDate: nullTS(v.Date),
		})
	})
}

func (r *CustomFieldRepo) ValuesByTask(ctx context.Context, orgID, taskID string) ([]project.CustomValue, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("custom value list: %w", err)
	}
	out := make([]project.CustomValue, 0)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListTaskCustomValues(ctx, gen.ListTaskCustomValuesParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.CustomValue{
				FieldID: row.FieldID.String(), Name: row.Name, Type: row.Type,
				Text: row.ValueText, Number: numericToFloat(row.ValueNumber), Date: tsPtr(row.ValueDate),
			})
		}
		return nil
	})
	return out, err
}

func (r *CustomFieldRepo) ClearValue(ctx context.Context, orgID, taskID, fieldID string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("custom value clear: task: %w", err)
	}
	fid, err := parseUUID(fieldID)
	if err != nil {
		return fmt.Errorf("custom value clear: field: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteTaskCustomValue(ctx, gen.DeleteTaskCustomValueParams{OrgID: oid, TaskID: tid, FieldID: fid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
