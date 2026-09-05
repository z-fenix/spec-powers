package property

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// ---- fakes ----

type fakeProperties struct {
	defs     map[string]*domain.PropertyDefinition
	values   map[string]*domain.IssuePropertyValue // "issue|property"
	issues   *fakeIssueStore
	nextID   int
	conflict bool
}

func newFakeProperties(issues *fakeIssueStore) *fakeProperties {
	return &fakeProperties{
		defs:   map[string]*domain.PropertyDefinition{},
		values: map[string]*domain.IssuePropertyValue{},
		issues: issues,
	}
}

func (f *fakeProperties) CreatePropertyDefinition(_ context.Context, d *domain.PropertyDefinition) (*domain.PropertyDefinition, error) {
	if f.conflict {
		return nil, store.ErrConflict
	}
	f.nextID++
	clone := *d
	clone.ID = string(rune('P' + f.nextID))
	f.defs[clone.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeProperties) GetPropertyDefinition(_ context.Context, id string) (*domain.PropertyDefinition, error) {
	d, ok := f.defs[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *d
	return &out, nil
}

func (f *fakeProperties) ListPropertyDefinitions(_ context.Context, projectID string) ([]domain.PropertyDefinition, error) {
	var out []domain.PropertyDefinition
	for _, d := range f.defs {
		if d.ProjectID == projectID {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (f *fakeProperties) UpdatePropertyDefinition(_ context.Context, d *domain.PropertyDefinition) (*domain.PropertyDefinition, error) {
	if _, ok := f.defs[d.ID]; !ok {
		return nil, store.ErrNotFound
	}
	clone := *d
	f.defs[d.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeProperties) DeletePropertyDefinition(_ context.Context, id string) error {
	if _, ok := f.defs[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.defs, id)
	for k, v := range f.values {
		if v.PropertyID == id {
			delete(f.values, k)
		}
	}
	return nil
}

func (f *fakeProperties) SetIssueProperty(_ context.Context, v *domain.IssuePropertyValue) (*domain.IssuePropertyValue, error) {
	if _, ok := f.defs[v.PropertyID]; !ok {
		return nil, store.ErrNotFound
	}
	clone := *v
	f.values[v.IssueID+"|"+v.PropertyID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeProperties) ListIssueProperties(_ context.Context, issueID string) ([]domain.IssuePropertyValue, error) {
	var out []domain.IssuePropertyValue
	for _, v := range f.values {
		if v.IssueID == issueID {
			out = append(out, *v)
		}
	}
	return out, nil
}

func (f *fakeProperties) ListIssuePropertiesForProject(_ context.Context, projectID string) ([]domain.IssuePropertyValue, error) {
	var out []domain.IssuePropertyValue
	for _, v := range f.values {
		if issue, ok := f.issues.byID[v.IssueID]; ok && issue.ProjectID == projectID {
			out = append(out, *v)
		}
	}
	return out, nil
}

func (f *fakeProperties) DeleteIssueProperty(_ context.Context, issueID, propertyID string) error {
	key := issueID + "|" + propertyID
	if _, ok := f.values[key]; !ok {
		return store.ErrNotFound
	}
	delete(f.values, key)
	return nil
}

type fakeProjects struct {
	store.ProjectStore
	existing map[string]bool
	members  map[string]string // "project|user" -> role
}

func (f *fakeProjects) GetProject(_ context.Context, id string) (*domain.Project, error) {
	if !f.existing[id] {
		return nil, store.ErrNotFound
	}
	return &domain.Project{ID: id}, nil
}

func (f *fakeProjects) GetProjectMember(_ context.Context, projectID, userID string) (*domain.ProjectMember, error) {
	role, ok := f.members[projectID+"|"+userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.ProjectMember{ProjectID: projectID, UserID: userID, Role: role}, nil
}

type fakeIssueStore struct {
	store.IssueStore
	byID map[string]*domain.Issue
}

func (f *fakeIssueStore) GetIssue(_ context.Context, id string) (*domain.Issue, error) {
	i, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *i
	return &out, nil
}

// ---- fixtures ----

func newService() (*Service, *fakeProperties, *fakeIssueStore) {
	issues := &fakeIssueStore{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", ProjectID: "p1", Title: "one"},
		"i2": {ID: "i2", ProjectID: "p2", Title: "two"},
	}}
	props := newFakeProperties(issues)
	projects := &fakeProjects{
		existing: map[string]bool{"p1": true, "p2": true},
		members: map[string]string{
			"p1|alice": "owner",
			"p1|bob":   "member",
			"p2|alice": "owner",
		},
	}
	return NewService(props, projects, issues), props, issues
}

func mustStatus(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with status %d, got nil", status)
	}
	appErr, ok := err.(*httpapi.AppError)
	if !ok || appErr.Status != status {
		t.Fatalf("error = %v, want status %d", err, status)
	}
}

// ---- definition tests ----

func TestCreateDefinition(t *testing.T) {
	svc, props, _ := newService()

	t.Run("owner creates a select property", func(t *testing.T) {
		d, err := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{
			Name: "优先级来源", Type: TypeSelect, Options: []string{"产品", "研发"},
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if d.ID == "" || d.Position != 0 || len(d.Options) != 2 {
			t.Errorf("created = %+v", d)
		}
	})

	t.Run("position increments per project", func(t *testing.T) {
		d, err := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "估算", Type: TypeNumber})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if d.Position != 1 {
			t.Errorf("position = %d, want 1", d.Position)
		}
	})

	t.Run("member is forbidden", func(t *testing.T) {
		_, err := svc.CreateDefinition(context.Background(), "bob", "p1", DefinitionInput{Name: "x", Type: TypeText})
		mustStatus(t, err, 403)
	})

	t.Run("validation errors", func(t *testing.T) {
		cases := []struct {
			name string
			in   DefinitionInput
		}{
			{"blank name", DefinitionInput{Name: "  ", Type: TypeText}},
			{"bad type", DefinitionInput{Name: "x", Type: "enum"}},
			{"select without options", DefinitionInput{Name: "x", Type: TypeSelect}},
			{"duplicate options", DefinitionInput{Name: "x", Type: TypeSelect, Options: []string{"a", "a"}}},
			{"blank option", DefinitionInput{Name: "x", Type: TypeSelect, Options: []string{"a", " "}}},
			{"options on text", DefinitionInput{Name: "x", Type: TypeText, Options: []string{"a"}}},
		}
		for _, c := range cases {
			_, err := svc.CreateDefinition(context.Background(), "alice", "p1", c.in)
			mustStatus(t, err, 400)
		}
	})

	t.Run("name conflict maps to 409", func(t *testing.T) {
		props.conflict = true
		defer func() { props.conflict = false }()
		_, err := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "dup", Type: TypeText})
		mustStatus(t, err, 409)
	})
}

func TestUpdateAndDeleteDefinition(t *testing.T) {
	svc, _, _ := newService()
	d, err := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{
		Name: "状态来源", Type: TypeSelect, Options: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("owner updates", func(t *testing.T) {
		up, err := svc.UpdateDefinition(context.Background(), "alice", "p1", d.ID, DefinitionInput{
			Name: "状态来源2", Type: TypeSelect, Options: []string{"a", "b", "c"},
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if up.Name != "状态来源2" || len(up.Options) != 3 {
			t.Errorf("updated = %+v", up)
		}
	})

	t.Run("cross-project update is 400", func(t *testing.T) {
		_, err := svc.UpdateDefinition(context.Background(), "alice", "p2", d.ID, DefinitionInput{Name: "x", Type: TypeText})
		mustStatus(t, err, 400)
	})

	t.Run("unknown property is 404", func(t *testing.T) {
		_, err := svc.UpdateDefinition(context.Background(), "alice", "p1", "nope", DefinitionInput{Name: "x", Type: TypeText})
		mustStatus(t, err, 404)
	})

	t.Run("delete removes the definition", func(t *testing.T) {
		if err := svc.DeleteDefinition(context.Background(), "alice", "p1", d.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		err := svc.DeleteDefinition(context.Background(), "alice", "p1", d.ID)
		mustStatus(t, err, 404)
	})
}

// ---- value tests ----

func TestSetIssueValue(t *testing.T) {
	svc, _, _ := newService()
	select_, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{
		Name: "模块", Type: TypeSelect, Options: []string{"前端", "后端"},
	})
	multi, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{
		Name: "标签", Type: TypeMultiSelect, Options: []string{"x", "y"},
	})
	text, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "备注", Type: TypeText})

	t.Run("member sets a select value", func(t *testing.T) {
		v, err := svc.SetIssueValue(context.Background(), "bob", "i1", select_.ID, "后端")
		if err != nil {
			t.Fatalf("set: %v", err)
		}
		if v.Value != "后端" || v.IssueID != "i1" || v.PropertyID != select_.ID {
			t.Errorf("value = %+v", v)
		}
	})

	t.Run("select value outside options is 400", func(t *testing.T) {
		_, err := svc.SetIssueValue(context.Background(), "bob", "i1", select_.ID, "测试")
		mustStatus(t, err, 400)
	})

	t.Run("multi_select takes a JSON array", func(t *testing.T) {
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", multi.ID, `["x","y"]`); err != nil {
			t.Fatalf("set multi: %v", err)
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", multi.ID, `["z"]`); err == nil {
			t.Error("expected 400 for option outside the list")
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", multi.ID, "x"); err == nil {
			t.Error("expected 400 for non-JSON value")
		}
	})

	t.Run("typed value validation", func(t *testing.T) {
		num, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "点数", Type: TypeNumber})
		check, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "完成", Type: TypeCheckbox})
		date, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "到期", Type: TypeDate})

		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", num.ID, "3.5"); err != nil {
			t.Errorf("number: %v", err)
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", num.ID, "abc"); err == nil {
			t.Error("expected 400 for non-number")
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", check.ID, "true"); err != nil {
			t.Errorf("checkbox: %v", err)
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", check.ID, "yes"); err == nil {
			t.Error("expected 400 for non-bool")
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", date.ID, "2026-09-05"); err != nil {
			t.Errorf("date: %v", err)
		}
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", date.ID, "09/05/2026"); err == nil {
			t.Error("expected 400 for bad date")
		}
		_ = text
	})

	t.Run("empty value clears the assignment", func(t *testing.T) {
		if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", select_.ID, ""); err != nil {
			t.Fatalf("clear: %v", err)
		}
		list, err := svc.ListIssueValues(context.Background(), "bob", "i1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, v := range list {
			if v.PropertyID == select_.ID {
				t.Errorf("select value still present: %+v", v)
			}
		}
	})

	t.Run("cross-project property is 400", func(t *testing.T) {
		p2prop, err := svc.CreateDefinition(context.Background(), "alice", "p2", DefinitionInput{Name: "别的", Type: TypeText})
		if err != nil {
			t.Fatalf("create p2: %v", err)
		}
		_, err = svc.SetIssueValue(context.Background(), "bob", "i1", p2prop.ID, "x")
		mustStatus(t, err, 400)
	})

	t.Run("unknown issue or property is 404", func(t *testing.T) {
		_, err := svc.SetIssueValue(context.Background(), "bob", "nope", text.ID, "x")
		mustStatus(t, err, 404)
		_, err = svc.SetIssueValue(context.Background(), "bob", "i1", "nope", "x")
		mustStatus(t, err, 404)
	})

	t.Run("non-member is 403", func(t *testing.T) {
		_, err := svc.SetIssueValue(context.Background(), "mallory", "i1", text.ID, "x")
		mustStatus(t, err, 403)
	})
}

func TestListProjectValues(t *testing.T) {
	svc, _, _ := newService()
	text, _ := svc.CreateDefinition(context.Background(), "alice", "p1", DefinitionInput{Name: "备注", Type: TypeText})
	if _, err := svc.SetIssueValue(context.Background(), "bob", "i1", text.ID, "hello"); err != nil {
		t.Fatalf("set: %v", err)
	}
	p2text, err := svc.CreateDefinition(context.Background(), "alice", "p2", DefinitionInput{Name: "备注", Type: TypeText})
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}
	if _, err := svc.SetIssueValue(context.Background(), "alice", "i2", p2text.ID, "other project"); err != nil {
		t.Fatalf("set p2: %v", err)
	}

	list, err := svc.ListProjectValues(context.Background(), "bob", "p1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].IssueID != "i1" || list[0].Value != "hello" {
		t.Errorf("list = %+v, want only i1/hello", list)
	}

	if _, err := svc.ListProjectValues(context.Background(), "mallory", "p1"); err == nil {
		t.Error("expected 403 for non-member")
	}
}
