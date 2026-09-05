package agent

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
	"specpowers/backend/internal/workflow"
)

type fakeChangeStore struct {
	store.ChangeStore
	byID    map[string]*domain.Change
	byIssue map[string]*domain.Change
	nextID  int
}

func (f *fakeChangeStore) CreateChange(_ context.Context, c *domain.Change) (*domain.Change, error) {
	f.nextID++
	cp := *c
	cp.ID = "c-new-" + string(rune('0'+f.nextID))
	f.byID[cp.ID] = &cp
	f.byIssue[cp.IssueID] = &cp
	return &cp, nil
}

func (f *fakeChangeStore) GetChange(_ context.Context, id string) (*domain.Change, error) {
	c, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (f *fakeChangeStore) GetChangeByIssue(_ context.Context, issueID string) (*domain.Change, error) {
	c, ok := f.byIssue[issueID]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

type fakeArtifactStore struct {
	store.ArtifactStore
	next int
}

func (f *fakeArtifactStore) CreateArtifact(_ context.Context, a *domain.Artifact) (*domain.Artifact, error) {
	f.next++
	cp := *a
	cp.ID = "art-new"
	cp.Version = f.next
	return &cp, nil
}

type fakeMappingStore struct {
	store.TaskMappingStore
}

func (f *fakeMappingStore) SetTaskMappings(_ context.Context, _, _ string, _ []domain.TaskMapping) error {
	return nil
}

func TestWorkflowFlowEnsureChange(t *testing.T) {
	ctx := context.Background()
	changes := &fakeChangeStore{byID: map[string]*domain.Change{}, byIssue: map[string]*domain.Change{}}
	artifacts := &fakeArtifactStore{}
	mappings := &fakeMappingStore{}
	issues := &fakeIssueStore{issue: &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}}
	projects := &fakeProjectStore{}

	reg, err := skill.NewRegistry(skillFS(map[string]string{
		"skills/brainstorm.md": "---\nkey: brainstorm\nname: Brainstorm\ndescription: d\norder: 1\n---\nINSTR",
	}))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	svc := workflow.NewService(changes, artifacts, mappings, issues, projects).
		WithSkills(reg).
		WithAgentAccess(agentLookup{agents: map[string]bool{"agent-1": true}})
	flow := NewWorkflowFlow(svc)

	t.Run("creates a proposal-phase change when none exists", func(t *testing.T) {
		c, err := flow.EnsureChange(ctx, "agent-1", "i1")
		if err != nil {
			t.Fatalf("ensure change: %v", err)
		}
		if c.Phase != "proposal" || c.Status != "active" || c.IssueID != "i1" {
			t.Fatalf("change = %+v", c)
		}
	})

	t.Run("reuses the existing change on the next call", func(t *testing.T) {
		existing, _ := flow.EnsureChange(ctx, "agent-1", "i1")
		again, err := flow.EnsureChange(ctx, "agent-1", "i1")
		if err != nil {
			t.Fatalf("second ensure: %v", err)
		}
		if again.ID != existing.ID {
			t.Fatalf("second ensure created a new change: %s vs %s", again.ID, existing.ID)
		}
	})

	t.Run("phase skill resolves for the active change", func(t *testing.T) {
		c, _ := flow.EnsureChange(ctx, "agent-1", "i1")
		sk, err := flow.PhaseSkill(ctx, "agent-1", c)
		if err != nil {
			t.Fatalf("phase skill: %v", err)
		}
		if sk.Key != "brainstorm" {
			t.Fatalf("skill = %+v", sk)
		}
	})

	t.Run("write artifact stores a versioned artifact", func(t *testing.T) {
		c, _ := flow.EnsureChange(ctx, "agent-1", "i1")
		a, err := flow.WriteArtifact(ctx, "agent-1", c, "proposal", "# Proposal", "")
		if err != nil {
			t.Fatalf("write artifact: %v", err)
		}
		if a.Kind != "proposal" || a.Version != 1 {
			t.Fatalf("artifact = %+v", a)
		}
	})
}

type agentLookup struct {
	agents map[string]bool
}

func (a agentLookup) IsAgent(_ context.Context, userID string) bool {
	return a.agents[userID]
}
