package issue

import (
	"context"
	"sort"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// fakeStatusStore mirrors the postgres store's semantics: nil (or empty)
// stored rows mean the built-in defaults are still in effect, and the first
// mutation materializes them.
type fakeStatusStore struct {
	rows map[string][]domain.WorkspaceStatus // workspaceID -> stored entries
}

func newFakeStatusStore() *fakeStatusStore {
	return &fakeStatusStore{rows: map[string][]domain.WorkspaceStatus{}}
}

func (f *fakeStatusStore) sorted(wsID string) []domain.WorkspaceStatus {
	out := append([]domain.WorkspaceStatus{}, f.rows[wsID]...)
	sort.SliceStable(out, func(a, b int) bool { return out[a].Position < out[b].Position })
	return out
}

// ensureSeeded materializes the defaults before a mutation, like the
// postgres store.
func (f *fakeStatusStore) ensureSeeded(wsID string) {
	if len(f.rows[wsID]) > 0 {
		return
	}
	f.rows[wsID] = domain.DefaultStatusDirectory()
}

func (f *fakeStatusStore) ListStatuses(_ context.Context, wsID string) ([]domain.WorkspaceStatus, error) {
	if len(f.rows[wsID]) == 0 {
		return domain.DefaultStatusDirectory(), nil
	}
	return f.sorted(wsID), nil
}

func (f *fakeStatusStore) UpsertStatus(_ context.Context, s *domain.WorkspaceStatus) (*domain.WorkspaceStatus, error) {
	f.ensureSeeded(s.WorkspaceID)
	for i, r := range f.rows[s.WorkspaceID] {
		if r.Name == s.Name {
			f.rows[s.WorkspaceID][i].Category = s.Category
			f.rows[s.WorkspaceID][i].Position = s.Position
			return &f.rows[s.WorkspaceID][i], nil
		}
	}
	f.rows[s.WorkspaceID] = append(f.rows[s.WorkspaceID], *s)
	return s, nil
}

func (f *fakeStatusStore) DeleteStatus(_ context.Context, wsID, name string) error {
	f.ensureSeeded(wsID)
	for i, r := range f.rows[wsID] {
		if r.Name == name {
			f.rows[wsID] = append(f.rows[wsID][:i], f.rows[wsID][i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

// newDirectoryService wires a service whose project p1 belongs to workspace
// w1 with a status store.
func newDirectoryService() (*Service, *fakeStatusStore, *fakeIssues) {
	issues := newFakeIssues()
	projects := &fakeProjects{
		existing:   map[string]bool{"p1": true},
		members:    map[string]string{"p1|alice": "owner", "p1|bob": "member"},
		workspaces: map[string]string{"p1": "w1"},
	}
	users := &fakeUsers{ids: map[string]bool{}, emails: map[string]string{}}
	statuses := newFakeStatusStore()
	svc := NewService(issues, projects, users).WithStatusStore(statuses)
	return svc, statuses, issues
}

func findStatus(list []domain.WorkspaceStatus, name string) (domain.WorkspaceStatus, bool) {
	for _, s := range list {
		if s.Name == name {
			return s, true
		}
	}
	return domain.WorkspaceStatus{}, false
}

func TestListStatusesDefaults(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newDirectoryService()

	t.Run("returns the built-in seven for an untouched workspace", func(t *testing.T) {
		list, err := svc.ListStatuses(ctx, "alice", "p1")
		if err != nil {
			t.Fatalf("ListStatuses: %v", err)
		}
		if len(list) != 7 {
			t.Fatalf("len = %d, want 7 built-in statuses", len(list))
		}
		for i, s := range list {
			if s.Name != s.Category || s.Position != i {
				t.Errorf("entry %d = %+v, want name==category and ordered position", i, s)
			}
		}
	})

	t.Run("requires project membership", func(t *testing.T) {
		_, err := svc.ListStatuses(ctx, "mallory", "p1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})

	t.Run("unknown project is 404", func(t *testing.T) {
		_, err := svc.ListStatuses(ctx, "alice", "p2")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}

func TestUpsertStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("adds a custom status mapped to a category", func(t *testing.T) {
		svc, statuses, _ := newDirectoryService()
		list, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "qa_review", Category: domain.CatInReview})
		if err != nil {
			t.Fatalf("UpsertStatus: %v", err)
		}
		// defaults materialized + the new entry appended
		if len(list) != 8 {
			t.Fatalf("len = %d, want 8", len(list))
		}
		entry, ok := findStatus(list, "qa_review")
		if !ok || entry.Category != domain.CatInReview {
			t.Fatalf("qa_review entry = %+v, ok = %v", entry, ok)
		}
		if len(statuses.rows["w1"]) != 8 {
			t.Errorf("stored rows = %d, want 8 (defaults materialized)", len(statuses.rows["w1"]))
		}
	})

	t.Run("updates an existing entry", func(t *testing.T) {
		svc, statuses, _ := newDirectoryService()
		if _, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "todo", Category: domain.CatTodo, Position: intPtr(0)}); err != nil {
			t.Fatalf("setup: %v", err)
		}
		list, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "todo", Category: domain.CatBacklog, Position: intPtr(0)})
		if err != nil {
			t.Fatalf("UpsertStatus: %v", err)
		}
		entry, _ := findStatus(list, "todo")
		if entry.Category != domain.CatBacklog {
			t.Errorf("todo category = %q, want backlog", entry.Category)
		}
		if len(statuses.rows["w1"]) != 7 {
			t.Errorf("stored rows = %d, want 7 (update, not insert)", len(statuses.rows["w1"]))
		}
	})

	t.Run("owner only", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		_, err := svc.UpsertStatus(ctx, "bob", "p1", StatusInput{Name: "x", Category: domain.CatTodo})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})

	t.Run("validates name and category", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		if _, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "  ", Category: domain.CatTodo}); err == nil {
			t.Error("blank name should be rejected")
		}
		_, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "x", Category: "almost_done"})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})
}

func TestDeleteStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("removes a built-in status", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		list, err := svc.DeleteStatus(ctx, "alice", "p1", "blocked")
		if err != nil {
			t.Fatalf("DeleteStatus: %v", err)
		}
		if len(list) != 6 {
			t.Fatalf("len = %d, want 6", len(list))
		}
		if _, ok := findStatus(list, "blocked"); ok {
			t.Error("blocked should be gone from the directory")
		}
	})

	t.Run("unknown status is 404", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		_, err := svc.DeleteStatus(ctx, "alice", "p1", "ghost")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})

	t.Run("owner only", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		_, err := svc.DeleteStatus(ctx, "bob", "p1", "blocked")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})
}

func TestIssueStateMachineFollowsDirectory(t *testing.T) {
	ctx := context.Background()

	t.Run("transition to a custom status is validated by category", func(t *testing.T) {
		svc, _, issues := newDirectoryService()
		if _, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "qa_review", Category: domain.CatInReview}); err != nil {
			t.Fatalf("setup: %v", err)
		}
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "x"})
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		// todo -> in_progress -> qa_review -> done follows the category
		// machine; todo -> qa_review (in_review) directly stays illegal.
		if _, err := svc.TransitionStatus(ctx, "alice", created.ID, "qa_review"); err == nil {
			t.Fatal("todo -> qa_review should be illegal (todo has no in_review transition)")
		}
		if _, err := svc.TransitionStatus(ctx, "alice", created.ID, "in_progress"); err != nil {
			t.Fatalf("todo -> in_progress should be legal: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", created.ID, "qa_review"); err != nil {
			t.Fatalf("in_progress -> qa_review should be legal: %v", err)
		}
		if issues.byID[created.ID].Status != "qa_review" {
			t.Errorf("stored status = %q, want qa_review", issues.byID[created.ID].Status)
		}
		// qa_review is in_review category: done is reachable, todo is not.
		if _, err := svc.TransitionStatus(ctx, "alice", created.ID, "done"); err != nil {
			t.Fatalf("qa_review -> done should be legal: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", created.ID, "todo"); err == nil {
			t.Fatal("done -> todo should be illegal")
		}
	})

	t.Run("statuses removed from the directory are rejected", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		if _, err := svc.DeleteStatus(ctx, "alice", "p1", "blocked"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "x"})
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		_, err = svc.TransitionStatus(ctx, "alice", created.ID, "blocked")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400 unknown status", err)
		}
	})

	t.Run("new issue defaults to the first todo-category status", func(t *testing.T) {
		svc, _, _ := newDirectoryService()
		if _, err := svc.UpsertStatus(ctx, "alice", "p1", StatusInput{Name: "ready", Category: domain.CatTodo, Position: intPtr(0)}); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := svc.DeleteStatus(ctx, "alice", "p1", "todo"); err != nil {
			t.Fatalf("delete todo: %v", err)
		}
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "x"})
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		if created.Status != "ready" {
			t.Errorf("status = %q, want ready (first todo-category entry)", created.Status)
		}
	})
}

func intPtr(i int) *int { return &i }
