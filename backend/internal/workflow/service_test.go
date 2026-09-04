package workflow

import (
	"context"
	"fmt"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// ---- fakes ----

type fakeChanges struct {
	byID       map[string]*domain.Change
	byIssue    map[string]*domain.Change
	handoffs   map[string][]domain.ChangeHandoff // changeID -> newest first
	handoffSeq int
	seq        int
}

func (f *fakeChanges) CreateChange(_ context.Context, c *domain.Change) (*domain.Change, error) {
	f.seq++
	stored := *c
	stored.ID = fmt.Sprintf("c-new-%d", f.seq)
	f.byID[stored.ID] = &stored
	f.byIssue[stored.IssueID] = &stored
	out := stored
	return &out, nil
}

func (f *fakeChanges) UpdateChange(_ context.Context, c *domain.Change) (*domain.Change, error) {
	stored, ok := f.byID[c.ID]
	if !ok {
		return nil, store.ErrNotFound
	}
	*stored = *c
	out := *c
	return &out, nil
}

func (f *fakeChanges) CreateChangeHandoff(_ context.Context, h *domain.ChangeHandoff) (*domain.ChangeHandoff, error) {
	out := *h
	f.handoffSeq++
	out.ID = fmt.Sprintf("h-%d", f.handoffSeq)
	f.handoffs[h.ChangeID] = append([]domain.ChangeHandoff{out}, f.handoffs[h.ChangeID]...)
	return &out, nil
}

func (f *fakeChanges) ListChangeHandoffs(_ context.Context, changeID string) ([]domain.ChangeHandoff, error) {
	list := f.handoffs[changeID]
	out := make([]domain.ChangeHandoff, len(list))
	copy(out, list)
	return out, nil
}

func (f *fakeChanges) GetChange(_ context.Context, id string) (*domain.Change, error) {
	c, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *c
	return &out, nil
}

func (f *fakeChanges) GetChangeByIssue(_ context.Context, issueID string) (*domain.Change, error) {
	c, ok := f.byIssue[issueID]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *c
	return &out, nil
}

type fakeArtifacts struct {
	latest  map[string][]domain.Artifact // changeID -> latest per kind
	byKind  map[string][]domain.Artifact // kind -> versions, newest first
	missing map[string]bool              // changeID|kind -> not found
}

func (f *fakeArtifacts) CreateArtifact(_ context.Context, a *domain.Artifact) (*domain.Artifact, error) {
	out := *a
	out.Version = 1
	if versions := f.byKind[a.Kind]; len(versions) > 0 {
		out.Version = versions[0].Version + 1
	}
	out.ID = fmt.Sprintf("a-%s-v%d", a.Kind, out.Version)
	f.byKind[a.Kind] = append([]domain.Artifact{out}, f.byKind[a.Kind]...)
	// keep the latest-per-kind list in sync
	latest := f.latest[a.ChangeID]
	replaced := false
	for i := range latest {
		if latest[i].Kind == a.Kind {
			latest[i] = out
			replaced = true
		}
	}
	if !replaced {
		f.latest[a.ChangeID] = append(latest, out)
	}
	return &out, nil
}

func (f *fakeArtifacts) GetArtifact(_ context.Context, changeID, kind string, version int) (*domain.Artifact, error) {
	if f.missing[changeID+"|"+kind] {
		return nil, store.ErrNotFound
	}
	for _, a := range f.byKind[kind] {
		if version <= 0 || a.Version == version {
			out := a
			return &out, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeArtifacts) ListArtifacts(_ context.Context, changeID string) ([]domain.Artifact, error) {
	list, ok := f.latest[changeID]
	if !ok {
		return nil, nil
	}
	out := make([]domain.Artifact, len(list))
	copy(out, list)
	return out, nil
}

func (f *fakeArtifacts) ListArtifactVersions(_ context.Context, changeID, kind string) ([]domain.Artifact, error) {
	list, ok := f.byKind[kind]
	if !ok {
		return nil, nil
	}
	out := make([]domain.Artifact, len(list))
	copy(out, list)
	return out, nil
}

type fakeMappings struct {
	byChange map[string][]domain.TaskMapping
}

func (f *fakeMappings) SetTaskMappings(_ context.Context, changeID, artifactID string, items []domain.TaskMapping) error {
	f.byChange[changeID] = items
	return nil
}

func (f *fakeMappings) ListTaskMappings(_ context.Context, changeID string) ([]domain.TaskMapping, error) {
	list, ok := f.byChange[changeID]
	if !ok {
		return nil, nil
	}
	out := make([]domain.TaskMapping, len(list))
	copy(out, list)
	return out, nil
}

type fakeIssues struct {
	byID map[string]*domain.Issue
}

func (f *fakeIssues) GetIssue(_ context.Context, id string) (*domain.Issue, error) {
	i, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *i
	return &out, nil
}

type fakeProjects struct {
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

// ---- fixture ----

type fixture struct {
	svc       *Service
	changes   *fakeChanges
	artifacts *fakeArtifacts
	mappings  *fakeMappings
	projects  *fakeProjects
	issues    *fakeIssues
}

func newFixture() *fixture {
	changes := &fakeChanges{byID: map[string]*domain.Change{}, byIssue: map[string]*domain.Change{},
		handoffs: map[string][]domain.ChangeHandoff{}}
	artifacts := &fakeArtifacts{latest: map[string][]domain.Artifact{}, byKind: map[string][]domain.Artifact{}, missing: map[string]bool{}}
	mappings := &fakeMappings{byChange: map[string][]domain.TaskMapping{}}
	projects := &fakeProjects{existing: map[string]bool{"p1": true}, members: map[string]string{
		"p1|alice": "owner",
		"p1|bob":   "member",
	}}
	issues := &fakeIssues{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", ProjectID: "p1"},
	}}
	svc := NewService(changes, artifacts, mappings, issues, projects)
	return &fixture{svc: svc, changes: changes, artifacts: artifacts, mappings: mappings, projects: projects, issues: issues}
}

func TestGetChange(t *testing.T) {
	f := newFixture()
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: "specs", Status: "active"}

	t.Run("member reads change", func(t *testing.T) {
		c, err := f.svc.GetChange(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("get change: %v", err)
		}
		if c.ID != "c1" || c.Phase != "specs" || c.Status != "active" {
			t.Errorf("change = %+v", c)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.GetChange(context.Background(), "eve", "c1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})

	t.Run("unknown change is not found", func(t *testing.T) {
		_, err := f.svc.GetChange(context.Background(), "bob", "nope")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})
}

func TestGetChangeByIssue(t *testing.T) {
	f := newFixture()
	f.changes.byIssue["i1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1"}

	t.Run("member reads change of issue", func(t *testing.T) {
		c, err := f.svc.GetChangeByIssue(context.Background(), "bob", "i1")
		if err != nil || c.ID != "c1" {
			t.Errorf("change = %+v, %v", c, err)
		}
	})

	t.Run("issue without change is not found", func(t *testing.T) {
		f.issues.byID["i2"] = &domain.Issue{ID: "i2", ProjectID: "p1"}
		_, err := f.svc.GetChangeByIssue(context.Background(), "bob", "i2")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})

	t.Run("unknown issue is not found", func(t *testing.T) {
		_, err := f.svc.GetChangeByIssue(context.Background(), "bob", "nope")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.GetChangeByIssue(context.Background(), "eve", "i1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})
}

func TestListArtifacts(t *testing.T) {
	f := newFixture()
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1"}
	f.artifacts.latest = map[string][]domain.Artifact{"c1": {
		{ID: "a1", ChangeID: "c1", Kind: "proposal", Version: 2, Content: "# v2"},
		{ID: "a2", ChangeID: "c1", Kind: "tasks", Version: 1, Content: "# tasks"},
	}}

	t.Run("member lists latest per kind", func(t *testing.T) {
		list, err := f.svc.ListArtifacts(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 2 || list[0].Kind != "proposal" || list[0].Version != 2 {
			t.Errorf("list = %+v", list)
		}
	})

	t.Run("no artifacts returns empty", func(t *testing.T) {
		f.changes.byID["c2"] = &domain.Change{ID: "c2", ProjectID: "p1"}
		list, err := f.svc.ListArtifacts(context.Background(), "bob", "c2")
		if err != nil || list != nil {
			t.Errorf("list = %+v, %v, want nil", list, err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.ListArtifacts(context.Background(), "eve", "c1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})
}

func TestGetArtifact(t *testing.T) {
	f := newFixture()
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1"}
	f.artifacts.byKind["proposal"] = []domain.Artifact{
		{ID: "a1", ChangeID: "c1", Kind: "proposal", Version: 2, Content: "# v2"},
		{ID: "a0", ChangeID: "c1", Kind: "proposal", Version: 1, Content: "# v1"},
	}

	t.Run("latest when version omitted", func(t *testing.T) {
		a, err := f.svc.GetArtifact(context.Background(), "bob", "c1", "proposal", 0)
		if err != nil || a.Version != 2 {
			t.Errorf("artifact = %+v, %v", a, err)
		}
	})

	t.Run("specific version", func(t *testing.T) {
		a, err := f.svc.GetArtifact(context.Background(), "bob", "c1", "proposal", 1)
		if err != nil || a.Version != 1 || a.Content != "# v1" {
			t.Errorf("artifact = %+v, %v", a, err)
		}
	})

	t.Run("unknown kind is invalid", func(t *testing.T) {
		_, err := f.svc.GetArtifact(context.Background(), "bob", "c1", "plan", 0)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Errorf("error = %v, want 400 invalid kind", err)
		}
	})

	t.Run("missing artifact is not found", func(t *testing.T) {
		f.artifacts.missing["c1|design"] = true
		_, err := f.svc.GetArtifact(context.Background(), "bob", "c1", "design", 0)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.GetArtifact(context.Background(), "eve", "c1", "proposal", 0)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})
}

func TestListTaskMappings(t *testing.T) {
	f := newFixture()
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1"}
	f.mappings.byChange["c1"] = []domain.TaskMapping{
		{ID: "m1", ChangeID: "c1", IssueID: "i1", Title: "child", Stage: 1, Position: 0},
	}

	t.Run("member lists mappings", func(t *testing.T) {
		list, err := f.svc.ListTaskMappings(context.Background(), "bob", "c1")
		if err != nil || len(list) != 1 || list[0].IssueID != "i1" {
			t.Errorf("list = %+v, %v", list, err)
		}
	})

	t.Run("no mappings returns empty", func(t *testing.T) {
		f.changes.byID["c2"] = &domain.Change{ID: "c2", ProjectID: "p1"}
		list, err := f.svc.ListTaskMappings(context.Background(), "bob", "c2")
		if err != nil || list != nil {
			t.Errorf("list = %+v, %v, want nil", list, err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.ListTaskMappings(context.Background(), "eve", "c1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})
}
