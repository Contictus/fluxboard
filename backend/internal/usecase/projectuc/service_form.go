package projectuc

import (
	"context"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/automation"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/rank"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
)

// ---- Intake forms (ADR-023) --------------------------------------------------
// A project LEAD publishes a shareable form; anyone with the token URL files a
// task into the form's target column without an account. The raw token is
// returned once at create/rotate (only its SHA-256 is stored). Anonymous tasks
// are attributed to the form owner (tasks.created_by is NOT NULL + FK) with
// the submitter stamped into the description.

// CreateFormInput carries the intake form editor.
type CreateFormInput struct {
	Name           string
	Description    string
	TargetColumnID string
}

// UpdateFormInput rewrites a form (all fields + kill switch).
type UpdateFormInput struct {
	Name           string
	Description    string
	TargetColumnID string
	IsActive       bool
}

// SubmitFormInput is one anonymous intake submission.
type SubmitFormInput struct {
	Title          string
	Description    string
	Priority       string
	SubmitterName  string
	SubmitterEmail string
}

// MaxSubmitterLen bounds the free-text submitter attribution.
const MaxSubmitterLen = 120

// formsEnabled reports whether the form repository is wired.
func (s *Service) formsEnabled() bool { return s.forms != nil }

// CreateForm stores a form and returns it with the one-time raw token (LEAD).
func (s *Service) CreateForm(ctx context.Context, orgID, userID, projectID string, in CreateFormInput, orgRole tenant.OrgRole) (*project.ProjectForm, string, error) {
	if !s.formsEnabled() {
		return nil, "", domain.ErrForbidden // forms disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, "", err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > project.MaxFormNameLen {
		return nil, "", domain.ErrValidation
	}
	if err := s.requireColumn(ctx, orgID, userID, projectID, in.TargetColumnID, orgRole); err != nil {
		return nil, "", err
	}
	raw, err := token.New()
	if err != nil {
		return nil, "", err
	}
	f := &project.ProjectForm{
		ID: newID(), OrgID: orgID, ProjectID: projectID, Name: name,
		Description: strings.TrimSpace(in.Description), TargetColumnID: in.TargetColumnID,
		TokenHash: token.Hash(raw), CreatedBy: userID, IsActive: true,
	}
	if err := s.forms.Create(ctx, orgID, f); err != nil {
		return nil, "", err
	}
	stored, err := s.forms.Get(ctx, orgID, f.ID)
	if err != nil {
		return nil, "", err
	}
	return stored, raw, nil
}

// ListForms returns a project's forms oldest-first (VIEWER). Token hashes
// never leave the repo — the raw token exists only at create/rotate.
func (s *Service) ListForms(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) ([]project.ProjectForm, error) {
	if !s.formsEnabled() {
		return nil, domain.ErrForbidden // forms disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.forms.ListByProject(ctx, orgID, projectID)
}

// UpdateForm rewrites a form (LEAD).
func (s *Service) UpdateForm(ctx context.Context, orgID, userID, projectID, formID string, in UpdateFormInput, orgRole tenant.OrgRole) (*project.ProjectForm, error) {
	if !s.formsEnabled() {
		return nil, domain.ErrForbidden // forms disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > project.MaxFormNameLen {
		return nil, domain.ErrValidation
	}
	if err := s.requireColumn(ctx, orgID, userID, projectID, in.TargetColumnID, orgRole); err != nil {
		return nil, err
	}
	f, err := s.formInProject(ctx, orgID, projectID, formID)
	if err != nil {
		return nil, err
	}
	f.Name = name
	f.Description = strings.TrimSpace(in.Description)
	f.TargetColumnID = in.TargetColumnID
	f.IsActive = in.IsActive
	if err := s.forms.Update(ctx, orgID, f); err != nil {
		return nil, err
	}
	return s.forms.Get(ctx, orgID, formID)
}

// RotateFormToken swaps a form's token, killing the old URL (LEAD). Returns
// the new one-time raw token.
func (s *Service) RotateFormToken(ctx context.Context, orgID, userID, projectID, formID string, orgRole tenant.OrgRole) (string, error) {
	if !s.formsEnabled() {
		return "", domain.ErrForbidden // forms disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return "", err
	}
	if _, err := s.formInProject(ctx, orgID, projectID, formID); err != nil {
		return "", err
	}
	raw, err := token.New()
	if err != nil {
		return "", err
	}
	if err := s.forms.RotateToken(ctx, orgID, formID, token.Hash(raw)); err != nil {
		return "", err
	}
	return raw, nil
}

// DeleteForm removes a form; submitted tasks are untouched (LEAD).
func (s *Service) DeleteForm(ctx context.Context, orgID, userID, projectID, formID string, orgRole tenant.OrgRole) error {
	if !s.formsEnabled() {
		return domain.ErrForbidden // forms disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	if _, err := s.formInProject(ctx, orgID, projectID, formID); err != nil {
		return err
	}
	return s.forms.Delete(ctx, orgID, formID)
}

// formInProject loads a form, ensuring it belongs to the project.
func (s *Service) formInProject(ctx context.Context, orgID, projectID, formID string) (*project.ProjectForm, error) {
	f, err := s.forms.Get(ctx, orgID, formID)
	if err != nil {
		return nil, err
	}
	if f.ProjectID != projectID {
		return nil, domain.ErrNotFound
	}
	return f, nil
}

// ---- Public surface (no auth — the token is the credential) ------------------

// PublicForm is the safe projection the anonymous form page renders.
type PublicForm struct {
	Name        string
	Description string
}

// GetPublicForm resolves the form page data. Unknown, disabled, or archived
// forms all read as NotFound so the token can't be used as an oracle.
func (s *Service) GetPublicForm(ctx context.Context, rawToken string) (*PublicForm, error) {
	if !s.formsEnabled() {
		return nil, domain.ErrNotFound
	}
	f, err := s.forms.ResolveByToken(ctx, token.Hash(rawToken))
	if err != nil {
		return nil, domain.ErrNotFound // never leak which tokens exist
	}
	if !f.IsActive {
		return nil, domain.ErrNotFound
	}
	p, err := s.projects.Get(ctx, f.OrgID, f.ProjectID)
	if err != nil || p.Archived() {
		return nil, domain.ErrNotFound
	}
	return &PublicForm{Name: f.Name, Description: f.Description}, nil
}

// SubmitForm files an anonymous task through the form. Returns the created
// task so the page can confirm receipt (number for human reference).
func (s *Service) SubmitForm(ctx context.Context, rawToken string, in SubmitFormInput) (*project.Task, error) {
	if !s.formsEnabled() {
		return nil, domain.ErrNotFound
	}
	f, err := s.forms.ResolveByToken(ctx, token.Hash(rawToken))
	if err != nil {
		return nil, domain.ErrNotFound // never leak which tokens exist
	}
	if !f.IsActive {
		return nil, domain.ErrNotFound
	}
	p, err := s.projects.Get(ctx, f.OrgID, f.ProjectID)
	if err != nil || p.Archived() {
		return nil, domain.ErrNotFound
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > project.MaxTitleLen {
		return nil, domain.ErrValidation
	}
	priority := project.Priority(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = project.PriorityNone
	}
	if !priority.Valid() {
		return nil, domain.ErrValidation
	}
	name := strings.TrimSpace(in.SubmitterName)
	email := strings.TrimSpace(in.SubmitterEmail)
	if len(name) > MaxSubmitterLen || len(email) > MaxSubmitterLen {
		return nil, domain.ErrValidation
	}
	if email != "" && !strings.Contains(email, "@") {
		return nil, domain.ErrValidation
	}
	col, err := s.columns.Get(ctx, f.OrgID, f.TargetColumnID)
	if err != nil {
		return nil, domain.ErrValidation // target column gone — owner must re-point the form
	}
	board, err := s.boards.GetByProject(ctx, f.OrgID, f.ProjectID)
	if err != nil {
		return nil, domain.ErrValidation
	}
	if col.BoardID != board.ID {
		return nil, domain.ErrValidation
	}
	existing, err := s.tasks.ListByColumn(ctx, f.OrgID, f.TargetColumnID)
	if err != nil {
		return nil, err
	}
	last := ""
	if n := len(existing); n > 0 {
		last = existing[n-1].Rank
	}
	t := &project.Task{
		ID: newID(), OrgID: f.OrgID, ProjectID: f.ProjectID, ColumnID: f.TargetColumnID,
		Title: title, Description: intakeBody(name, email, strings.TrimSpace(in.Description)),
		Priority: priority, Rank: rank.Append(last), CreatedBy: f.CreatedBy,
	}
	if err := s.tasks.Create(ctx, f.OrgID, t); err != nil {
		return nil, err
	}
	s.publish(ctx, f.OrgID, notify.EventTaskCreated, f.CreatedBy, map[string]any{
		"task_id": t.ID, "project_id": t.ProjectID, "column_id": t.ColumnID, "title": t.Title,
	})
	s.automate(ctx, automation.Event{
		OrgID: f.OrgID, Trigger: automation.TriggerTaskCreated, TaskID: t.ID, ActorID: f.CreatedBy,
	})
	return s.tasks.Get(ctx, f.OrgID, t.ID)
}

// intakeBody stamps the anonymous submitter into the description so the owner
// can follow up (tasks.created_by points at the form owner instead).
func intakeBody(name, email, body string) string {
	var who string
	switch {
	case name != "" && email != "":
		who = name + " <" + email + ">"
	case name != "":
		who = name
	case email != "":
		who = email
	}
	if who == "" {
		return body
	}
	if body == "" {
		return "Submitted via public form by " + who
	}
	return "Submitted via public form by " + who + "\n\n---\n\n" + body
}
