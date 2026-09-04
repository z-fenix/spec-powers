package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
)

func skillFS(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, content := range files {
		out[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return out
}

// ---- fakes ----

type fakeLLM struct {
	responses []string // consumed in order; the last one repeats
	requests  []llmRequest
	err       error
}

type llmRequest struct {
	system string
	user   string
}

func (f *fakeLLM) Complete(_ context.Context, system, user string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.requests = append(f.requests, llmRequest{system: system, user: user})
	if len(f.responses) == 0 {
		return "", fmt.Errorf("fakeLLM: no scripted responses")
	}
	resp := f.responses[0]
	if len(f.responses) > 1 {
		f.responses = f.responses[1:]
	}
	return resp, nil
}

type fakeIssueStore struct {
	store.IssueStore
	issue   *domain.Issue
	extra   map[string]*domain.Issue
	updated []*domain.Issue
}

func (f *fakeIssueStore) GetIssue(_ context.Context, id string) (*domain.Issue, error) {
	if f.issue != nil && f.issue.ID == id {
		cp := *f.issue
		return &cp, nil
	}
	if i, ok := f.extra[id]; ok {
		cp := *i
		return &cp, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeIssueStore) UpdateIssue(_ context.Context, i *domain.Issue) (*domain.Issue, error) {
	cp := *i
	if f.issue != nil && f.issue.ID == i.ID {
		f.issue = &cp
	} else if _, ok := f.extra[i.ID]; ok {
		f.extra[i.ID] = &cp
	}
	f.updated = append(f.updated, &cp)
	return &cp, nil
}

type fakeCommentStore struct {
	store.CommentStore
	created []*domain.IssueComment
}

func (f *fakeCommentStore) CreateComment(_ context.Context, c *domain.IssueComment) (*domain.IssueComment, error) {
	cp := *c
	cp.ID = fmt.Sprintf("comment-%d", len(f.created)+1)
	f.created = append(f.created, &cp)
	return &cp, nil
}

func (f *fakeCommentStore) ListComments(_ context.Context, _ string) ([]domain.IssueComment, error) {
	var out []domain.IssueComment
	for _, c := range f.created {
		out = append(out, *c)
	}
	return out, nil
}

type fakeMetadataStore struct {
	store.IssueMetadataStore
	items []domain.IssueMetadata
}

func (f *fakeMetadataStore) ListIssueMetadata(_ context.Context, issueID string) ([]domain.IssueMetadata, error) {
	return f.items, nil
}

type fakeProjectStore struct {
	store.ProjectStore
	resources []domain.ProjectResource
}

func (f *fakeProjectStore) ListProjectResources(_ context.Context, projectID string) ([]domain.ProjectResource, error) {
	return f.resources, nil
}

type fakeCheckout struct {
	calls []checkoutCall
	err   error
}

type checkoutCall struct {
	pointer string
	destDir string
}

func (f *fakeCheckout) Checkout(_ context.Context, pointer, destDir string) error {
	f.calls = append(f.calls, checkoutCall{pointer: pointer, destDir: destDir})
	return f.err
}

// ---- helpers ----

func testSkillRegistry() (*skill.Registry, error) {
	return skill.NewRegistry(skillFS(map[string]string{
		"skills/brainstorm.md": "---\nkey: brainstorm\nname: Brainstorm\ndescription: explore intent\norder: 1\n---\nBRAINSTORM_INSTRUCTIONS",
	}))
}

func newExecutorForTest(t *testing.T, client llm.Client, issue *domain.Issue) (*Executor, *fakeIssueStore, *fakeCommentStore, *fakeCheckout, *fakeLogs) {
	t.Helper()
	reg, err := testSkillRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	issues := &fakeIssueStore{issue: issue}
	comments := &fakeCommentStore{}
	metadata := &fakeMetadataStore{items: []domain.IssueMetadata{{IssueID: issue.ID, Key: "k", Value: "v", Type: "string"}}}
	projects := &fakeProjectStore{resources: []domain.ProjectResource{
		{ID: "r1", ProjectID: issue.ProjectID, Type: "github_repo", Label: "comet", Pointer: "rpamis/comet"},
		{ID: "r2", ProjectID: issue.ProjectID, Type: "local_directory", Label: "spec-powers", Pointer: `D:\work\spec-powers`},
	}}
	checkout := &fakeCheckout{}
	logs := &fakeLogs{}
	e := NewExecutor(ExecutorDeps{
		Issues:   issues,
		Comments: comments,
		Metadata: metadata,
		Projects: projects,
		Client:   client,
		Skills:   reg,
		WorkDir:  "work",
		Checkout: checkout.Checkout,
		Logs:     logs,
		MaxTurns: 4,
	})
	return e, issues, comments, checkout, logs
}

func TestExecutorCommentsTriggerMentions(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"post_comment","args":{"content":"handing to @Reviewer"}}`,
		`{"action":"final","message":"done, thanks @Reviewer"}`,
	}}
	e, _, comments, _, _ := newExecutorForTest(t, client, issue)
	var mentioned []string
	e.mentionHook = func(ctx context.Context, issueID, authorID, content string) error {
		mentioned = append(mentioned, issueID+"|"+authorID+"|"+content)
		return nil
	}
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(mentioned) != 2 {
		t.Fatalf("mention hook calls = %d, want 2", len(mentioned))
	}
	if mentioned[0] != "i1|agent-1|handing to @Reviewer" {
		t.Fatalf("first hook = %q", mentioned[0])
	}
	if mentioned[1] != "i1|agent-1|done, thanks @Reviewer" {
		t.Fatalf("second hook = %q", mentioned[1])
	}
	_ = comments
}

// ---- tests ----

func TestExecutorLogsEveryTurn(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"read_issue","args":{}}`,
		`{"action":"final","message":"done"}`,
	}}
	e, _, _, _, logs := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var kinds []string
	for _, l := range logs.logs {
		if l.RunID != "run-1" {
			t.Fatalf("log for wrong run: %+v", l)
		}
		kinds = append(kinds, l.Kind)
	}
	want := []string{"llm_request", "llm_response", "tool_call", "tool_result", "llm_request", "llm_response"}
	if len(kinds) != len(want) {
		t.Fatalf("log kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("log kinds = %v, want %v", kinds, want)
		}
	}
}

func TestExecutorFinalReplyPostsComment(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "Do it", Status: "todo", AssigneeID: "agent-1"}
	client := &fakeLLM{responses: []string{`{"action":"final","message":"All done!"}`}}
	e, _, comments, _, _ := newExecutorForTest(t, client, issue)

	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1", Trigger: "assigned"}
	agent := &domain.Agent{ID: "agent-1", Name: "KunCoding", Description: "chief"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(comments.created) != 1 {
		t.Fatalf("comments = %d, want 1", len(comments.created))
	}
	c := comments.created[0]
	if c.IssueID != "i1" || c.AuthorID != "agent-1" || c.Content != "All done!" {
		t.Fatalf("comment = %+v", c)
	}

	// The system prompt carries the agent identity and the JSON protocol.
	if len(client.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(client.requests))
	}
	sys := client.requests[0].system
	if !strings.Contains(sys, "KunCoding") || !strings.Contains(sys, "read_issue") ||
		!strings.Contains(sys, "post_comment") || !strings.Contains(sys, "set_status") ||
		!strings.Contains(sys, "checkout_repo") {
		t.Fatalf("system prompt missing identity/tools: %q", sys)
	}
	// The user prompt carries the issue context.
	user := client.requests[0].user
	if !strings.Contains(user, "Do it") {
		t.Fatalf("user prompt missing issue title: %q", user)
	}
}

func TestExecutorSkillInstructionsInSystemPrompt(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{responses: []string{`{"action":"final","message":"ok"}`}}
	e, _, _, _, _ := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A", Skills: []string{"brainstorm"}}
	if err := e.Execute(context.Background(), &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(client.requests[0].system, "BRAINSTORM_INSTRUCTIONS") {
		t.Fatalf("system prompt missing skill instructions: %q", client.requests[0].system)
	}
}

func TestExecutorToolLoopFeedsResultBack(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "Loop me", Status: "todo"}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"read_issue","args":{}}`,
		`{"action":"final","message":"read it"}`,
	}}
	e, _, comments, _, _ := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	// The second request must contain the tool result (issue title + metadata).
	second := client.requests[1].user
	if !strings.Contains(second, "Loop me") || !strings.Contains(second, `"k"`) {
		t.Fatalf("second request missing tool result: %q", second)
	}
	// Only the final message becomes a comment.
	if len(comments.created) != 1 || comments.created[0].Content != "read it" {
		t.Fatalf("comments = %+v", comments.created)
	}
}

func TestExecutorPostCommentAndSetStatusTools(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T", Status: "todo"}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"post_comment","args":{"content":"working on it"}}`,
		`{"action":"tool","tool":"set_status","args":{"status":"in_progress"}}`,
		`{"action":"final","message":"started"}`,
	}}
	e, issues, comments, _, _ := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(comments.created) != 2 {
		t.Fatalf("comments = %d, want 2", len(comments.created))
	}
	if comments.created[0].Content != "working on it" || comments.created[0].AuthorID != "agent-1" {
		t.Fatalf("tool comment = %+v", comments.created[0])
	}
	if comments.created[1].Content != "started" {
		t.Fatalf("final comment = %+v", comments.created[1])
	}
	if len(issues.updated) != 1 || issues.updated[0].Status != "in_progress" {
		t.Fatalf("issue updates = %+v", issues.updated)
	}
}

func TestExecutorCheckoutRepoTool(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"checkout_repo","args":{}}`,
		`{"action":"final","message":"checked out"}`,
	}}
	e, _, _, checkout, _ := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-9", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(checkout.calls) != 1 {
		t.Fatalf("checkout calls = %d, want 1 (only the github repo)", len(checkout.calls))
	}
	if checkout.calls[0].pointer != "rpamis/comet" || !strings.Contains(checkout.calls[0].destDir, "run-9") {
		t.Fatalf("checkout call = %+v", checkout.calls[0])
	}
}

func TestExecutorBadJSONLoggedThenRecovers(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{responses: []string{
		`not json at all`,
		`{"action":"final","message":"recovered"}`,
	}}
	e, _, comments, _, _ := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(comments.created) != 1 || comments.created[0].Content != "recovered" {
		t.Fatalf("comments = %+v", comments.created)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
}

func TestExecutorMaxTurnsExhaustedFails(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"read_issue","args":{}}`,
	}}
	e, _, _, _, _ := newExecutorForTest(t, client, issue)
	e.MaxTurns = 2
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	err := e.Execute(context.Background(), run, agent)
	if err == nil || !strings.Contains(err.Error(), "max turns") {
		t.Fatalf("err = %v, want max turns exhausted", err)
	}
}

func TestExecutorLLMFailureReturnsError(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	client := &fakeLLM{err: fmt.Errorf("endpoint down")}
	e, _, _, _, _ := newExecutorForTest(t, client, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err == nil || !strings.Contains(err.Error(), "endpoint down") {
		t.Fatalf("err = %v, want endpoint down", err)
	}
}

func TestExecutorMissingIssueFails(t *testing.T) {
	reg, err := testSkillRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	e := NewExecutor(ExecutorDeps{
		Issues:   &fakeIssueStore{},
		Comments: &fakeCommentStore{},
		Metadata: &fakeMetadataStore{},
		Projects: &fakeProjectStore{},
		Client:   &fakeLLM{},
		Skills:   reg,
		MaxTurns: 4,
	})
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "missing"}
	if err := e.Execute(context.Background(), run, agent); err == nil {
		t.Fatalf("missing issue should fail")
	}
}
