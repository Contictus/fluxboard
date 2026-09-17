package projectuc

import (
	"context"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// ---- Custom fields (FR-FIELDS) ------------------------------------------------
// Project-scoped typed attributes. Definitions are managed by LEADs; values
// are set by CONTRIBUTORs and read by VIEWERs.

// CreateFieldInput carries the field builder form.
type CreateFieldInput struct {
	Name     string
	Type     string
	Options  []string
	Position int
}

// fieldsEnabled reports whether the custom-field repository is wired.
func (s *Service) fieldsEnabled() bool { return s.fields != nil }

// CreateField stores a field definition (LEAD).
func (s *Service) CreateField(ctx context.Context, orgID, userID, projectID string, in CreateFieldInput, orgRole tenant.OrgRole) (*project.CustomField, error) {
	if !s.fieldsEnabled() {
		return nil, domain.ErrForbidden // fields disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || !project.ValidFieldType(in.Type) {
		return nil, domain.ErrValidation
	}
	opts := cleanOptions(in.Options)
	if in.Type == project.FieldSelect && len(opts) == 0 {
		return nil, domain.ErrValidation
	}
	f := &project.CustomField{
		ID: newID(), OrgID: orgID, ProjectID: projectID, Name: name,
		Type: in.Type, Options: opts, Position: in.Position, CreatedBy: userID,
	}
	if err := s.fields.Create(ctx, orgID, f); err != nil {
		return nil, err
	}
	return s.fields.Get(ctx, orgID, f.ID)
}

// ListFields returns definitions in position order (VIEWER).
func (s *Service) ListFields(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) ([]project.CustomField, error) {
	if !s.fieldsEnabled() {
		return nil, domain.ErrForbidden // fields disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.fields.ListByProject(ctx, orgID, projectID)
}

// UpdateField renames, re-options, or repositions a field (LEAD). Type is
// immutable: values are typed, so changing it would orphan data.
func (s *Service) UpdateField(ctx context.Context, orgID, userID, projectID, fieldID, name string, options []string, position int, orgRole tenant.OrgRole) (*project.CustomField, error) {
	if !s.fieldsEnabled() {
		return nil, domain.ErrForbidden // fields disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	f, err := s.fieldInProject(ctx, orgID, projectID, fieldID)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ErrValidation
	}
	opts := cleanOptions(options)
	if f.Type == project.FieldSelect && len(opts) == 0 {
		return nil, domain.ErrValidation
	}
	f.Name = name
	f.Options = opts
	f.Position = position
	if err := s.fields.Update(ctx, orgID, f); err != nil {
		return nil, err
	}
	return f, nil
}

// DeleteField removes a definition; values cascade (LEAD).
func (s *Service) DeleteField(ctx context.Context, orgID, userID, projectID, fieldID string, orgRole tenant.OrgRole) error {
	if !s.fieldsEnabled() {
		return domain.ErrForbidden // fields disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	if _, err := s.fieldInProject(ctx, orgID, projectID, fieldID); err != nil {
		return err
	}
	return s.fields.Delete(ctx, orgID, fieldID)
}

// SetFieldValue validates the value against the field type and upserts it
// (CONTRIBUTOR).
func (s *Service) SetFieldValue(ctx context.Context, orgID, userID, taskID, fieldID string, v project.CustomValue, orgRole tenant.OrgRole) error {
	if !s.fieldsEnabled() {
		return domain.ErrForbidden // fields disabled
	}
	t, _, err := s.taskInOrg(ctx, orgID, taskID, userID, orgRole, project.RoleContributor)
	if err != nil {
		return err
	}
	f, err := s.fieldInProject(ctx, orgID, t.ProjectID, fieldID)
	if err != nil {
		return err
	}
	if err := checkFieldValue(f, v); err != nil {
		return err
	}
	return s.fields.SetValue(ctx, orgID, taskID, fieldID, v)
}

// TaskFieldValues returns a task's values in field order (VIEWER).
func (s *Service) TaskFieldValues(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.CustomValue, error) {
	if !s.fieldsEnabled() {
		return nil, domain.ErrForbidden // fields disabled
	}
	if _, _, err := s.taskInOrg(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.fields.ValuesByTask(ctx, orgID, taskID)
}

// ClearFieldValue removes one task value (CONTRIBUTOR).
func (s *Service) ClearFieldValue(ctx context.Context, orgID, userID, taskID, fieldID string, orgRole tenant.OrgRole) error {
	if !s.fieldsEnabled() {
		return domain.ErrForbidden // fields disabled
	}
	t, _, err := s.taskInOrg(ctx, orgID, taskID, userID, orgRole, project.RoleContributor)
	if err != nil {
		return err
	}
	if _, err := s.fieldInProject(ctx, orgID, t.ProjectID, fieldID); err != nil {
		return err
	}
	return s.fields.ClearValue(ctx, orgID, taskID, fieldID)
}

// fieldInProject loads a field, ensuring it belongs to the project.
func (s *Service) fieldInProject(ctx context.Context, orgID, projectID, fieldID string) (*project.CustomField, error) {
	f, err := s.fields.Get(ctx, orgID, fieldID)
	if err != nil {
		return nil, err
	}
	if f.ProjectID != projectID {
		return nil, domain.ErrNotFound
	}
	return f, nil
}

// taskInOrg loads a task with a project-role gate (shared by value endpoints).
func (s *Service) taskInOrg(ctx context.Context, orgID, taskID, userID string, orgRole tenant.OrgRole, min project.ProjectRole) (*project.Task, project.ProjectRole, error) {
	t, err := s.tasks.Get(ctx, orgID, taskID)
	if err != nil {
		return nil, "", err
	}
	_, role, err := s.access(ctx, orgID, t.ProjectID, userID, orgRole, min)
	if err != nil {
		return nil, "", err
	}
	return t, role, nil
}

func cleanOptions(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool)
	for _, o := range in {
		o = strings.TrimSpace(o)
		if o == "" || seen[o] {
			continue
		}
		seen[o] = true
		out = append(out, o)
		if len(out) >= 50 {
			break
		}
	}
	return out
}

func checkFieldValue(f *project.CustomField, v project.CustomValue) error {
	switch f.Type {
	case project.FieldText:
		if v.Text == nil || strings.TrimSpace(*v.Text) == "" {
			return domain.ErrValidation
		}
	case project.FieldNumber:
		if v.Number == nil {
			return domain.ErrValidation
		}
	case project.FieldDate:
		if v.Date == nil || v.Date.IsZero() {
			return domain.ErrValidation
		}
	case project.FieldSelect:
		if v.Text == nil {
			return domain.ErrValidation
		}
		for _, o := range f.Options {
			if o == *v.Text {
				return nil
			}
		}
		return domain.ErrValidation
	default:
		return domain.ErrValidation
	}
	return nil
}
