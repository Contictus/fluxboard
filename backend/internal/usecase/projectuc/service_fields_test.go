package projectuc

import (
	"context"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

type fakeFields struct {
	project.CustomFieldRepository
	fields map[string]*project.CustomField
	values map[string]map[string]project.CustomValue
}

func newFakeFields(fs ...*project.CustomField) *fakeFields {
	m := make(map[string]*project.CustomField)
	for _, f := range fs {
		m[f.ID] = f
	}
	return &fakeFields{fields: m, values: make(map[string]map[string]project.CustomValue)}
}

func (f *fakeFields) Get(_ context.Context, _, id string) (*project.CustomField, error) {
	fld, ok := f.fields[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *fld
	return &cp, nil
}

func (f *fakeFields) SetValue(_ context.Context, _, taskID, fieldID string, v project.CustomValue) error {
	if f.values[taskID] == nil {
		f.values[taskID] = make(map[string]project.CustomValue)
	}
	f.values[taskID][fieldID] = v
	return nil
}

type fakeFieldTasks struct {
	project.TaskRepository
	tasks map[string]*project.Task
}

func (f *fakeFieldTasks) Get(_ context.Context, _, id string) (*project.Task, error) {
	t, ok := f.tasks[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}

func fieldsTestService(fields *fakeFields, tasks map[string]*project.Task) *Service {
	lead := project.RoleLead
	return New(Deps{
		Projects: &fakeProjects{p: &project.Project{ID: "p1", Visibility: project.VisibilityOrg}},
		Members:  &fakeMembers{role: &lead},
		Tasks:    &fakeFieldTasks{tasks: tasks},
		Fields:   fields,
	})
}

func TestCreateField_RejectsBadType(t *testing.T) {
	s := fieldsTestService(newFakeFields(), nil)
	if _, err := s.CreateField(context.Background(), "o", "u", "p1", CreateFieldInput{Name: "x", Type: "nope"}, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if _, err := s.CreateField(context.Background(), "o", "u", "p1", CreateFieldInput{Name: "x", Type: project.FieldSelect}, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("err = %v, want ErrValidation for select without options", err)
	}
}

func TestSetFieldValue_SelectMustMatchOptions(t *testing.T) {
	fields := newFakeFields(&project.CustomField{ID: "f1", ProjectID: "p1", Name: "Size", Type: project.FieldSelect, Options: []string{"S", "M"}})
	s := fieldsTestService(fields, map[string]*project.Task{"t1": {ID: "t1", ProjectID: "p1"}})
	bad := "XL"
	if err := s.SetFieldValue(context.Background(), "o", "u", "t1", "f1", project.CustomValue{Text: &bad}, tenant.RoleMember); err != domain.ErrValidation {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	good := "M"
	if err := s.SetFieldValue(context.Background(), "o", "u", "t1", "f1", project.CustomValue{Text: &good}, tenant.RoleMember); err != nil {
		t.Fatalf("SetFieldValue: %v", err)
	}
	if fields.values["t1"]["f1"].Text == nil || *fields.values["t1"]["f1"].Text != "M" {
		t.Fatalf("value not stored: %+v", fields.values["t1"])
	}
}
