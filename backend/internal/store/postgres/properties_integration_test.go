package postgres

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

func TestPropertyStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)
	issues := NewIssueStore(pool)
	props := NewPropertyStore(pool)

	owner, err := users.CreateUser(ctx, uniqueEmail("prop-owner"), "h", "Owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	p, err := projects.CreateProject(ctx, ws.ID, "Props", "", owner.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := projects.AddProjectMember(ctx, p.ID, owner.ID, "owner"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	i, err := issues.CreateIssue(ctx, &domain.Issue{ProjectID: p.ID, Title: "one", CreatedBy: owner.ID})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	created, err := props.CreatePropertyDefinition(ctx, &domain.PropertyDefinition{
		ProjectID: p.ID, Name: "模块", Type: "select", Options: []string{"前端", "后端"}, Position: 0,
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	if created.ID == "" || len(created.Options) != 2 {
		t.Errorf("created = %+v", created)
	}

	if _, err := props.CreatePropertyDefinition(ctx, &domain.PropertyDefinition{
		ProjectID: p.ID, Name: "模块", Type: "text",
	}); !IsConflict(err) {
		t.Errorf("duplicate name error = %v, want conflict", err)
	}

	got, err := props.GetPropertyDefinition(ctx, created.ID)
	if err != nil || got.Name != "模块" {
		t.Errorf("get = %+v, %v", got, err)
	}

	updated, err := props.UpdatePropertyDefinition(ctx, &domain.PropertyDefinition{
		ID: created.ID, ProjectID: p.ID, Name: "模块2", Type: "select", Options: []string{"a", "b", "c"}, Position: 0,
	})
	if err != nil || updated.Name != "模块2" || len(updated.Options) != 3 {
		t.Errorf("update = %+v, %v", updated, err)
	}

	list, err := props.ListPropertyDefinitions(ctx, p.ID)
	if err != nil || len(list) != 1 || list[0].Name != "模块2" {
		t.Errorf("list = %+v, %v", list, err)
	}

	saved, err := props.SetIssueProperty(ctx, &domain.IssuePropertyValue{IssueID: i.ID, PropertyID: created.ID, Value: "后端"})
	if err != nil || saved.Value != "后端" {
		t.Errorf("set = %+v, %v", saved, err)
	}
	again, err := props.SetIssueProperty(ctx, &domain.IssuePropertyValue{IssueID: i.ID, PropertyID: created.ID, Value: "前端"})
	if err != nil || again.Value != "前端" {
		t.Errorf("upsert = %+v, %v", again, err)
	}

	vals, err := props.ListIssueProperties(ctx, i.ID)
	if err != nil || len(vals) != 1 || vals[0].Value != "前端" {
		t.Errorf("list issue values = %+v, %v", vals, err)
	}

	vals, err = props.ListIssuePropertiesForProject(ctx, p.ID)
	if err != nil || len(vals) != 1 || vals[0].IssueID != i.ID {
		t.Errorf("list project values = %+v, %v", vals, err)
	}

	if err := props.DeleteIssueProperty(ctx, i.ID, created.ID); err != nil {
		t.Fatalf("delete value: %v", err)
	}
	if err := props.DeleteIssueProperty(ctx, i.ID, created.ID); err != store.ErrNotFound {
		t.Errorf("delete again error = %v, want ErrNotFound", err)
	}

	if err := props.DeletePropertyDefinition(ctx, created.ID); err != nil {
		t.Fatalf("delete definition: %v", err)
	}
	if err := props.DeletePropertyDefinition(ctx, created.ID); err != store.ErrNotFound {
		t.Errorf("delete again error = %v, want ErrNotFound", err)
	}
}
