package workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/store"
)

// ---- splitter fakes ----

type fakeLLM struct {
	fn    func(call int, system, user string) (string, error)
	calls []string // user prompts, in call order
}

func (f *fakeLLM) Complete(_ context.Context, system, user string) (llm.Completion, error) {
	f.calls = append(f.calls, user)
	if f.fn == nil {
		return llm.Completion{Text: "ok"}, nil
	}
	text, err := f.fn(len(f.calls), system, user)
	return llm.Completion{Text: text}, err
}

type splitChanges struct {
	existingByIssue map[string]*domain.Change
	created         *domain.Change
	updated         []*domain.Change
	handoffs        []domain.ChangeHandoff
	handoffSeq      int
}

func (f *splitChanges) CreateChangeHandoff(_ context.Context, h *domain.ChangeHandoff) (*domain.ChangeHandoff, error) {
	out := *h
	f.handoffSeq++
	out.ID = fmt.Sprintf("h-%d", f.handoffSeq)
	f.handoffs = append(f.handoffs, out)
	return &out, nil
}

func (f *splitChanges) ListChangeHandoffs(_ context.Context, changeID string) ([]domain.ChangeHandoff, error) {
	return f.handoffs, nil
}

func (f *splitChanges) CreateChange(_ context.Context, c *domain.Change) (*domain.Change, error) {
	out := *c
	out.ID = "c-new"
	now := out.CreatedAt
	out.CreatedAt, out.UpdatedAt = now, now
	f.created = &out
	return &out, nil
}

func (f *splitChanges) UpdateChange(_ context.Context, c *domain.Change) (*domain.Change, error) {
	out := *c
	f.updated = append(f.updated, &out)
	return &out, nil
}

func (f *splitChanges) GetChange(_ context.Context, id string) (*domain.Change, error) {
	if f.created != nil && f.created.ID == id {
		out := *f.created
		return &out, nil
	}
	return nil, store.ErrNotFound
}

func (f *splitChanges) GetChangeByIssue(_ context.Context, issueID string) (*domain.Change, error) {
	if c, ok := f.existingByIssue[issueID]; ok {
		out := *c
		return &out, nil
	}
	return nil, store.ErrNotFound
}

type splitArtifacts struct {
	created []domain.Artifact
	nextID  int
}

func (f *splitArtifacts) CreateArtifact(_ context.Context, a *domain.Artifact) (*domain.Artifact, error) {
	f.nextID++
	out := *a
	out.ID = fmt.Sprintf("a-%d", f.nextID)
	out.Version = 1
	f.created = append(f.created, out)
	return &out, nil
}

func (f *splitArtifacts) GetArtifact(_ context.Context, changeID, kind string, version int) (*domain.Artifact, error) {
	return nil, store.ErrNotFound
}

func (f *splitArtifacts) ListArtifacts(_ context.Context, changeID string) ([]domain.Artifact, error) {
	return nil, nil
}

func (f *splitArtifacts) ListArtifactVersions(_ context.Context, changeID, kind string) ([]domain.Artifact, error) {
	return nil, nil
}

type splitMappings struct {
	byChange map[string][]domain.TaskMapping
}

func (f *splitMappings) SetTaskMappings(_ context.Context, changeID, artifactID string, items []domain.TaskMapping) error {
	if f.byChange == nil {
		f.byChange = map[string][]domain.TaskMapping{}
	}
	stored := make([]domain.TaskMapping, len(items))
	copy(stored, items)
	f.byChange[changeID] = stored
	return nil
}

func (f *splitMappings) ListTaskMappings(_ context.Context, changeID string) ([]domain.TaskMapping, error) {
	list := f.byChange[changeID]
	out := make([]domain.TaskMapping, len(list))
	copy(out, list)
	// mirror the store's ORDER BY stage, position
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && out[b].Stage < out[b-1].Stage; b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out, nil
}

type splitCreator struct {
	created []issue.CreateInput
	nextID  int
}

func (f *splitCreator) CreateIssue(_ context.Context, userID, projectID string, in issue.CreateInput) (*domain.Issue, error) {
	f.nextID++
	out := &domain.Issue{
		ID:        fmt.Sprintf("sub-%d", f.nextID),
		ProjectID: projectID,
		Title:     in.Title,
		Stage:     in.Stage,
		Position:  f.nextID - 1,
		CreatedBy: userID,
	}
	f.created = append(f.created, in)
	return out, nil
}

type splitContexts struct {
	content string
}

func (f *splitContexts) GetProjectContext(_ context.Context, projectID string) (*domain.ProjectContext, error) {
	return &domain.ProjectContext{ProjectID: projectID, Content: f.content}, nil
}

// ---- helpers ----

const tasksJSON = "```json\n{\"tasks\":[" +
	`{"title":"任务一","description":"先做一","stage":1},` +
	`{"title":"任务二","description":"再做二","stage":2}` +
	"]}\n```"

func newSplitter(t *testing.T, client llm.Client, changes *splitChanges, artifacts *splitArtifacts, mappings *splitMappings, creator *splitCreator) *Splitter {
	t.Helper()
	return NewSplitter(SplitterDeps{
		Client:    client,
		Changes:   changes,
		Artifacts: artifacts,
		Mappings:  mappings,
		Issues: &fakeIssues{byID: map[string]*domain.Issue{
			"i1": {ID: "i1", ProjectID: "p1", Title: "父 issue", Description: "父描述"},
		}},
		Creator:    creator,
		Contexts:   &splitContexts{content: "项目背景"},
		Templates:  defaultTemplates(),
		MaxRetries: 2,
	})
}

func stageOneLLM(tasksContent string) *fakeLLM {
	byCall := []string{"P", "S", "D", tasksContent}
	return &fakeLLM{fn: func(call int, _, _ string) (string, error) {
		return byCall[call-1], nil
	}}
}

// ---- tests ----

func TestSplitRunsAllPhasesAndStoresArtifacts(t *testing.T) {
	client := stageOneLLM(tasksJSON)
	changes, artifacts, mappings, creator := &splitChanges{}, &splitArtifacts{}, &splitMappings{}, &splitCreator{}
	s := newSplitter(t, client, changes, artifacts, mappings, creator)

	change, err := s.Run(context.Background(), "bob", "i1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if change.ID != "c-new" || change.IssueID != "i1" || change.ProjectID != "p1" || change.CreatedBy != "bob" {
		t.Errorf("change = %+v", change)
	}
	if len(artifacts.created) != 4 {
		t.Fatalf("artifacts created = %d, want 4", len(artifacts.created))
	}
	for i, kind := range []string{KindProposal, KindSpecs, KindDesign, KindTasks} {
		if artifacts.created[i].Kind != kind {
			t.Errorf("artifact[%d].kind = %q, want %q", i, artifacts.created[i].Kind, kind)
		}
		if artifacts.created[i].CreatedBy != "bob" {
			t.Errorf("artifact[%d].createdBy = %q, want bob", i, artifacts.created[i].CreatedBy)
		}
	}
	if change.Phase != KindTasks || change.Status != "active" {
		t.Errorf("final change phase/status = %q/%q, want tasks/active", change.Phase, change.Status)
	}
	if len(mappings.byChange[change.ID]) != 2 {
		t.Errorf("mappings = %d, want 2 (tasks step)", len(mappings.byChange[change.ID]))
	}
}

func TestSplitAdvancesChangePhase(t *testing.T) {
	client := stageOneLLM(tasksJSON)
	changes := &splitChanges{}
	s := newSplitter(t, client, changes, &splitArtifacts{}, &splitMappings{}, &splitCreator{})

	if _, err := s.Run(context.Background(), "bob", "i1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// phase updates: proposal->specs->design->tasks (3 updates), plus any
	// terminal update; the last recorded phase must be tasks.
	if len(changes.updated) == 0 {
		t.Fatal("no phase updates recorded")
	}
	last := changes.updated[len(changes.updated)-1]
	if last.Phase != KindTasks {
		t.Errorf("last phase update = %q, want tasks", last.Phase)
	}
}

func TestSplitWritesHandoffOnPhaseAdvance(t *testing.T) {
	client := stageOneLLM(tasksJSON)
	changes := &splitChanges{}
	s := newSplitter(t, client, changes, &splitArtifacts{}, &splitMappings{}, &splitCreator{})

	if _, err := s.Run(context.Background(), "bob", "i1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []struct{ from, to string }{
		{KindProposal, KindSpecs}, {KindSpecs, KindDesign}, {KindDesign, KindTasks},
	}
	if len(changes.handoffs) != len(want) {
		t.Fatalf("handoffs = %d, want %d (%+v)", len(changes.handoffs), len(want), changes.handoffs)
	}
	for i, w := range want {
		h := changes.handoffs[i]
		if h.FromPhase != w.from || h.ToPhase != w.to || h.CreatedBy != "bob" || h.ChangeID != "c-new" {
			t.Errorf("handoff[%d] = %+v, want %s->%s", i, h, w.from, w.to)
		}
	}
}

func TestSplitCreatesSubIssuesAndMappings(t *testing.T) {
	client := stageOneLLM(tasksJSON)
	changes, artifacts, mappings, creator := &splitChanges{}, &splitArtifacts{}, &splitMappings{}, &splitCreator{}
	s := newSplitter(t, client, changes, artifacts, mappings, creator)

	change, err := s.Run(context.Background(), "bob", "i1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(creator.created) != 2 {
		t.Fatalf("sub-issues created = %d, want 2", len(creator.created))
	}
	first := creator.created[0]
	if first.Title != "任务一" || first.Description != "先做一" || first.Stage != 1 || first.ParentID != "i1" {
		t.Errorf("first input = %+v", first)
	}
	if creator.created[1].Stage != 2 {
		t.Errorf("second stage = %d, want 2", creator.created[1].Stage)
	}
	var tasksArtifactID string
	for _, a := range artifacts.created {
		if a.Kind == KindTasks {
			tasksArtifactID = a.ID
		}
	}
	if len(mappings.byChange[change.ID]) != 2 {
		t.Fatalf("SetTaskMappings items = %d, want 2", len(mappings.byChange[change.ID]))
	}
	call := struct {
		changeID, artifactID string
		items                []domain.TaskMapping
	}{change.ID, tasksArtifactID, mappings.byChange[change.ID]}
	if call.changeID != change.ID {
		t.Errorf("mapping changeID = %q, want %q", call.changeID, change.ID)
	}
	if call.artifactID != tasksArtifactID {
		t.Errorf("mapping artifactID = %q, want %q", call.artifactID, tasksArtifactID)
	}
	if len(call.items) != 2 {
		t.Fatalf("mapping items = %d, want 2", len(call.items))
	}
	if call.items[0].IssueID != "sub-1" || call.items[0].Stage != 1 || call.items[0].Position != 0 {
		t.Errorf("items[0] = %+v", call.items[0])
	}
	if call.items[1].IssueID != "sub-2" || call.items[1].Stage != 2 || call.items[1].Position != 1 {
		t.Errorf("items[1] = %+v", call.items[1])
	}
	if call.items[0].Title != "任务一" {
		t.Errorf("items[0].title = %q", call.items[0].Title)
	}
}

func TestSplitPromptCarriesPriorArtifactsAndContext(t *testing.T) {
	client := stageOneLLM(tasksJSON)
	s := newSplitter(t, client, &splitChanges{}, &splitArtifacts{}, &splitMappings{}, &splitCreator{})

	if _, err := s.Run(context.Background(), "bob", "i1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.calls) != 4 {
		t.Fatalf("LLM calls = %d, want 4", len(client.calls))
	}
	if !strings.Contains(client.calls[0], "父 issue") || !strings.Contains(client.calls[0], "父描述") {
		t.Errorf("proposal prompt missing issue title/description: %q", client.calls[0])
	}
	if !strings.Contains(client.calls[0], "项目背景") {
		t.Errorf("proposal prompt missing project context: %q", client.calls[0])
	}
	if !strings.Contains(client.calls[2], "P") {
		t.Errorf("design prompt missing proposal content: %q", client.calls[2])
	}
	if !strings.Contains(client.calls[3], "D") {
		t.Errorf("tasks prompt missing design content: %q", client.calls[3])
	}
}

func TestSplitRejectsDuplicateChange(t *testing.T) {
	changes := &splitChanges{existingByIssue: map[string]*domain.Change{
		"i1": {ID: "c-old", IssueID: "i1"},
	}}
	s := newSplitter(t, stageOneLLM(tasksJSON), changes, &splitArtifacts{}, &splitMappings{}, &splitCreator{})

	_, err := s.Run(context.Background(), "bob", "i1")
	appErr, ok := err.(*httpapi.AppError)
	if !ok || appErr.Status != 409 {
		t.Fatalf("err = %v, want 409 conflict", err)
	}
}

func TestSplitIssueNotFound(t *testing.T) {
	s := newSplitter(t, stageOneLLM(tasksJSON), &splitChanges{}, &splitArtifacts{}, &splitMappings{}, &splitCreator{})
	_, err := s.Run(context.Background(), "bob", "missing")
	if appErr, ok := err.(*httpapi.AppError); !ok || appErr.Status != 404 {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestSplitRetriesInvalidTasksOutput(t *testing.T) {
	calls := 0
	client := &fakeLLM{fn: func(call int, _, _ string) (string, error) {
		calls++
		if call <= 3 {
			return "P", nil
		}
		if call == 4 {
			return "not valid tasks output", nil
		}
		return tasksJSON, nil
	}}
	artifacts, mappings, creator := &splitArtifacts{}, &splitMappings{}, &splitCreator{}
	s := newSplitter(t, client, &splitChanges{}, artifacts, mappings, creator)

	if _, err := s.Run(context.Background(), "bob", "i1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 5 {
		t.Errorf("LLM calls = %d, want 5 (one invalid, one retry)", calls)
	}
	var tasksCount int
	for _, a := range artifacts.created {
		if a.Kind == KindTasks {
			tasksCount++
		}
	}
	if tasksCount != 1 {
		t.Errorf("tasks artifacts = %d, want 1", tasksCount)
	}
	if len(creator.created) != 2 {
		t.Errorf("sub-issues = %d, want 2", len(creator.created))
	}
}

func TestSplitFailsWhenRetriesExhausted(t *testing.T) {
	client := &fakeLLM{fn: func(int, string, string) (string, error) {
		return "", fmt.Errorf("llm down")
	}}
	changes, artifacts := &splitChanges{}, &splitArtifacts{}
	s := newSplitter(t, client, changes, artifacts, &splitMappings{}, &splitCreator{})

	_, err := s.Run(context.Background(), "bob", "i1")
	if err == nil {
		t.Fatal("expected error when LLM always fails")
	}
	if changes.created == nil {
		t.Fatal("change should have been created before failing")
	}
	if changes.created.Status != "failed" {
		t.Errorf("change status = %q, want failed", changes.created.Status)
	}
	if len(artifacts.created) != 0 {
		t.Errorf("artifacts = %d, want 0", len(artifacts.created))
	}
	if last := changes.updated[len(changes.updated)-1]; last.Status != "failed" {
		t.Errorf("last update status = %q, want failed", last.Status)
	}
}

func TestSplitFailsWhenOutputInvalidAfterRetries(t *testing.T) {
	client := &fakeLLM{fn: func(int, string, string) (string, error) {
		return "   ", nil // blank output is invalid for every phase
	}}
	changes, artifacts := &splitChanges{}, &splitArtifacts{}
	s := newSplitter(t, client, changes, artifacts, &splitMappings{}, &splitCreator{})

	if _, err := s.Run(context.Background(), "bob", "i1"); err == nil {
		t.Fatal("expected error on persistently invalid output")
	}
	if changes.created.Status != "failed" {
		t.Errorf("change status = %q, want failed", changes.created.Status)
	}
	if len(artifacts.created) != 0 {
		t.Errorf("artifacts = %d, want 0", len(artifacts.created))
	}
}
