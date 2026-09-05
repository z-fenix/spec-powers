package issue

import (
	"context"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// ---- fakes ----

type fakeIssues struct {
	byID     map[string]*domain.Issue
	nextID   int
	wakeups  map[string][]string // parentID -> childIssueIDs
	byWakeup map[string]bool     // "parent|child"
	children map[string][]string // parentID -> childIDs in insertion order
	comments map[string][]string // issueID -> comment contents
}

func newFakeIssues() *fakeIssues {
	return &fakeIssues{
		byID:     map[string]*domain.Issue{},
		wakeups:  map[string][]string{},
		byWakeup: map[string]bool{},
		children: map[string][]string{},
		comments: map[string][]string{},
	}
}

func (f *fakeIssues) CreateIssue(_ context.Context, i *domain.Issue) (*domain.Issue, error) {
	f.nextID++
	clone := *i
	clone.ID = string(rune('A' + f.nextID))
	clone.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clone.UpdatedAt = clone.CreatedAt
	f.byID[clone.ID] = &clone
	if clone.ParentID != "" {
		f.children[clone.ParentID] = append(f.children[clone.ParentID], clone.ID)
	}
	out := clone
	return &out, nil
}

func (f *fakeIssues) GetIssue(_ context.Context, id string) (*domain.Issue, error) {
	i, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *i
	return &out, nil
}

func (f *fakeIssues) UpdateIssue(_ context.Context, i *domain.Issue) (*domain.Issue, error) {
	stored, ok := f.byID[i.ID]
	if !ok {
		return nil, store.ErrNotFound
	}
	if stored.ParentID != i.ParentID {
		if stored.ParentID != "" {
			kids := f.children[stored.ParentID]
			for k, id := range kids {
				if id == i.ID {
					f.children[stored.ParentID] = append(kids[:k], kids[k+1:]...)
					break
				}
			}
		}
		if i.ParentID != "" {
			f.children[i.ParentID] = append(f.children[i.ParentID], i.ID)
		}
	}
	clone := *i
	clone.CreatedAt = stored.CreatedAt
	clone.UpdatedAt = clone.CreatedAt
	f.byID[i.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeIssues) DeleteIssue(_ context.Context, id string) error {
	if _, ok := f.byID[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeIssues) ListIssues(_ context.Context, projectID string, filter store.IssueFilter) ([]domain.Issue, error) {
	var out []domain.Issue
	for _, i := range f.byID {
		if i.ProjectID != projectID {
			continue
		}
		if filter.ParentID != nil && i.ParentID != *filter.ParentID {
			continue
		}
		if filter.Status != "" && i.Status != filter.Status {
			continue
		}
		if filter.Stage != nil && i.Stage != *filter.Stage {
			continue
		}
		if filter.Query != "" && !fakeIssueMatches(i, f.comments[i.ID], filter.Query) {
			continue
		}
		out = append(out, *i)
	}
	// deterministic order for tests: by ID
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && out[b].ID < out[b-1].ID; b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out, nil
}

// fakeIssueMatches mirrors the store-level keyword search: case-insensitive
// substring match on title, description and comment content.
func fakeIssueMatches(i *domain.Issue, comments []string, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(i.Title), q) || strings.Contains(strings.ToLower(i.Description), q) {
		return true
	}
	for _, c := range comments {
		if strings.Contains(strings.ToLower(c), q) {
			return true
		}
	}
	return false
}

func (f *fakeIssues) NextIssuePosition(_ context.Context, projectID, parentID string, stage int) (int, error) {
	max := -1
	for _, i := range f.byID {
		if i.ProjectID == projectID && i.ParentID == parentID && i.Stage == stage && i.Position > max {
			max = i.Position
		}
	}
	return max + 1, nil
}

func (f *fakeIssues) CreateIssueWakeup(_ context.Context, issueID, childIssueID string) error {
	key := issueID + "|" + childIssueID
	if f.byWakeup[key] {
		return nil
	}
	f.byWakeup[key] = true
	f.wakeups[issueID] = append(f.wakeups[issueID], childIssueID)
	return nil
}

func (f *fakeIssues) ListIssueWakeups(_ context.Context, issueID string) ([]domain.IssueWakeup, error) {
	var out []domain.IssueWakeup
	for _, child := range f.wakeups[issueID] {
		out = append(out, domain.IssueWakeup{IssueID: issueID, ChildIssueID: child})
	}
	return out, nil
}

type fakeProjects struct {
	store.ProjectStore
	existing   map[string]bool
	members    map[string]string // "project|user" -> role
	workspaces map[string]string // project -> workspaceID
}

func (f *fakeProjects) GetProject(_ context.Context, id string) (*domain.Project, error) {
	if !f.existing[id] {
		return nil, store.ErrNotFound
	}
	return &domain.Project{ID: id, WorkspaceID: f.workspaces[id]}, nil
}

func (f *fakeProjects) GetProjectMember(_ context.Context, projectID, userID string) (*domain.ProjectMember, error) {
	role, ok := f.members[projectID+"|"+userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.ProjectMember{ProjectID: projectID, UserID: userID, Role: role}, nil
}

type fakeUsers struct {
	store.UserStore
	emails map[string]string // email -> id
	ids    map[string]bool
}

func (f *fakeUsers) GetUser(_ context.Context, id string) (*domain.User, error) {
	if !f.ids[id] {
		return nil, store.ErrNotFound
	}
	return &domain.User{ID: id}, nil
}

func (f *fakeUsers) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	id, ok := f.emails[email]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.User{ID: id}, nil
}

// ---- helpers ----

func newService() (*Service, *fakeIssues, *fakeProjects, *fakeUsers) {
	issues := newFakeIssues()
	projects := &fakeProjects{
		existing: map[string]bool{"p1": true},
		members: map[string]string{
			"p1|alice": "owner",
			"p1|bob":   "member",
		},
	}
	users := &fakeUsers{ids: map[string]bool{"carol": true}, emails: map[string]string{}}
	return NewService(issues, projects, users), issues, projects, users
}

// ---- CreateIssue ----

func TestCreateIssue(t *testing.T) {
	ctx := context.Background()

	t.Run("creates a root issue with defaults", func(t *testing.T) {
		svc, _, _, _ := newService()
		in := CreateInput{Title: "Ship it", Description: "do the thing"}
		i, err := svc.CreateIssue(ctx, "alice", "p1", in)
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		if i.Title != "Ship it" || i.Description != "do the thing" {
			t.Errorf("title/description not persisted: %+v", i)
		}
		if i.ProjectID != "p1" {
			t.Errorf("ProjectID = %q, want p1", i.ProjectID)
		}
		if i.Status != StatusTodo {
			t.Errorf("Status = %q, want todo", i.Status)
		}
		if i.Priority != PriorityNone {
			t.Errorf("Priority = %q, want none", i.Priority)
		}
		if i.CreatedBy != "alice" {
			t.Errorf("CreatedBy = %q, want alice", i.CreatedBy)
		}
	})

	t.Run("requires a project member", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.CreateIssue(ctx, "mallory", "p1", CreateInput{Title: "x"})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403 forbidden", err)
		}
	})

	t.Run("rejects blank title", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "   "})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400 invalid", err)
		}
	})

	t.Run("rejects unknown priority", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "x", Priority: "asap"})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400 invalid", err)
		}
	})

	t.Run("rejects unknown assignee", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "x", AssigneeID: "ghost"})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})

	t.Run("accepts valid priority and assignee", func(t *testing.T) {
		svc, _, _, _ := newService()
		i, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "x", Priority: PriorityHigh, AssigneeID: "carol"})
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		if i.Priority != PriorityHigh || i.AssigneeID != "carol" {
			t.Errorf("priority/assignee not persisted: %+v", i)
		}
	})

	t.Run("unknown project is 404", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.CreateIssue(ctx, "alice", "p2", CreateInput{Title: "x"})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}

// ---- GetIssue / UpdateIssue / DeleteIssue ----

func TestGetIssue(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newService()
	created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("member can read", func(t *testing.T) {
		i, err := svc.GetIssue(ctx, "bob", created.ID)
		if err != nil {
			t.Fatalf("GetIssue: %v", err)
		}
		if i.ID != created.ID || i.Title != "a" {
			t.Errorf("got %+v", i)
		}
	})

	t.Run("stranger is forbidden", func(t *testing.T) {
		_, err := svc.GetIssue(ctx, "mallory", created.ID)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		_, err := svc.GetIssue(ctx, "alice", "missing")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}

func TestUpdateIssue(t *testing.T) {
	ctx := context.Background()

	t.Run("updates editable fields", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		title := "renamed"
		desc := "new desc"
		prio := PriorityUrgent
		due := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		in := UpdateInput{Title: &title, Description: &desc, Priority: &prio, AssigneeID: ptrString("carol"), DueDate: &due, Labels: []string{"backend", "api"}}
		i, err := svc.UpdateIssue(ctx, "bob", created.ID, in)
		if err != nil {
			t.Fatalf("UpdateIssue: %v", err)
		}
		if i.Title != "renamed" || i.Description != "new desc" || i.Priority != PriorityUrgent {
			t.Errorf("fields not updated: %+v", i)
		}
		if i.AssigneeID != "carol" || i.DueDate == nil || !i.DueDate.Equal(due) {
			t.Errorf("assignee/due not updated: %+v", i)
		}
		if len(i.Labels) != 2 || i.Labels[0] != "backend" {
			t.Errorf("labels not updated: %v", i.Labels)
		}
	})

	t.Run("nil fields are left unchanged", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a", Priority: PriorityLow})
		_, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{})
		if err != nil {
			t.Fatalf("UpdateIssue: %v", err)
		}
		i, _ := svc.GetIssue(ctx, "bob", created.ID)
		if i.Title != "a" || i.Priority != PriorityLow {
			t.Errorf("fields changed: %+v", i)
		}
	})

	t.Run("rejects blank title", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		blank := " "
		_, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{Title: &blank})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})

	t.Run("rejects unknown priority", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		prio := "asap"
		_, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{Priority: &prio})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})

	t.Run("rejects unknown assignee", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		_, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{AssigneeID: ptrString("ghost")})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}

func TestDeleteIssue(t *testing.T) {
	ctx := context.Background()

	t.Run("member can delete own project issue", func(t *testing.T) {
		svc, issues, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if err := svc.DeleteIssue(ctx, "bob", created.ID); err != nil {
			t.Fatalf("DeleteIssue: %v", err)
		}
		if _, err := issues.GetIssue(ctx, created.ID); err != store.ErrNotFound {
			t.Fatalf("issue still present: %v", err)
		}
	})

	t.Run("stranger is forbidden", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		err := svc.DeleteIssue(ctx, "mallory", created.ID)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		svc, _, _, _ := newService()
		err := svc.DeleteIssue(ctx, "alice", "missing")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}

// ---- ListIssues ----

func TestListIssues(t *testing.T) {
	ctx := context.Background()
	svc, issues, _, _ := newService()
	a, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
	b, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "b"})
	other, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "other"})
	// move b under a as stage 1, and mark other as in_progress directly.
	if _, err := svc.UpdateIssue(ctx, "alice", b.ID, UpdateInput{ParentID: ptrString(a.ID), Stage: ptrInt(1)}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	issues.byID[other.ID].Status = StatusInProgress

	t.Run("lists project issues for members", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("len = %d, want 3", len(list))
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{Status: StatusInProgress})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != other.ID {
			t.Fatalf("got %+v, want only %s", list, other.ID)
		}
	})

	t.Run("filters roots only", func(t *testing.T) {
		roots := ""
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{ParentID: &roots})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("len = %d, want 2 roots", len(list))
		}
	})

	t.Run("filters by stage", func(t *testing.T) {
		stage := 1
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{Stage: &stage})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != b.ID {
			t.Fatalf("got %+v, want only %s", list, b.ID)
		}
	})
}

func ptrString(s string) *string { return &s }
func ptrInt(i int) *int          { return &i }
