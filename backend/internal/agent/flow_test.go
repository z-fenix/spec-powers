package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/skill"
)

// ---- fake FlowDriver ----

type fakeFlow struct {
	change *domain.Change // nil = no change yet
	skill  *skill.Skill

	ensureCalled  []string // "userID|issueID" per call
	writes        []writeCall
	advanceCalled int
	advanceErr    error
	verifyCalls   []string
	archiveCalled int
	advanceResult *domain.Change
}

type writeCall struct {
	kind    string
	content string
	runID   string
}

func (f *fakeFlow) EnsureChange(_ context.Context, userID, issueID string) (*domain.Change, error) {
	f.ensureCalled = append(f.ensureCalled, userID+"|"+issueID)
	if f.change == nil {
		return nil, fmt.Errorf("fakeFlow: change unavailable")
	}
	return f.change, nil
}

func (f *fakeFlow) PhaseSkill(_ context.Context, _ string, _ *domain.Change) (*skill.Skill, error) {
	if f.skill == nil {
		return nil, fmt.Errorf("no next skill")
	}
	return f.skill, nil
}

func (f *fakeFlow) WriteArtifact(_ context.Context, _ string, _ *domain.Change, kind, content, runID string) (*domain.Artifact, error) {
	f.writes = append(f.writes, writeCall{kind: kind, content: content, runID: runID})
	return &domain.Artifact{ID: "art-1", ChangeID: f.change.ID, Kind: kind, Version: 1}, nil
}

func (f *fakeFlow) AdvancePhase(_ context.Context, _ string, _ *domain.Change) (*domain.Change, error) {
	f.advanceCalled++
	if f.advanceErr != nil {
		return nil, f.advanceErr
	}
	if f.advanceResult != nil {
		f.change = f.advanceResult
		return f.advanceResult, nil
	}
	return f.change, nil
}

func (f *fakeFlow) SubmitVerify(_ context.Context, _ string, _ *domain.Change, content, runID string) (*domain.Artifact, error) {
	f.verifyCalls = append(f.verifyCalls, content)
	return &domain.Artifact{ID: "verify-1", Kind: "verify", Version: 1}, nil
}

func (f *fakeFlow) Archive(_ context.Context, _ string, _ *domain.Change) (*domain.Change, error) {
	f.archiveCalled++
	f.change.Status = "archived"
	return f.change, nil
}

// ---- helpers ----

func flowExecutor(t *testing.T, client llmClient, flow *fakeFlow, issue *domain.Issue) (*Executor, *fakeIssueStore, *fakeLogs) {
	t.Helper()
	e, issues, _, _, logs := newExecutorForTest(t, client, issue)
	e.flow = flow
	return e, issues, logs
}

// llmClient is the local alias for the LLM interface used by test helpers.
type llmClient = interface {
	Complete(ctx context.Context, system, user string) (llm.Completion, error)
}

// ---- tests ----

func TestExecutorFlowEnsuresChangeAndInjectsSkill(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "Build it"}
	flow := &fakeFlow{
		change: &domain.Change{ID: "c1", IssueID: "i1", Phase: "proposal", Status: "active"},
		skill:  &skill.Skill{Key: "brainstorm", Name: "Brainstorm", Instructions: "BRAINSTORM_INSTRUCTIONS"},
	}
	client := &fakeLLM{responses: []string{`{"action":"final","message":"ok"}`}}
	e, _, _ := flowExecutor(t, client, flow, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(flow.ensureCalled) != 1 || flow.ensureCalled[0] != "agent-1|i1" {
		t.Fatalf("ensure calls = %v", flow.ensureCalled)
	}
	sys := client.requests[0].system
	if !strings.Contains(sys, "proposal") || !strings.Contains(sys, "c1") {
		t.Fatalf("system prompt missing change context: %q", sys)
	}
	if !strings.Contains(sys, "BRAINSTORM_INSTRUCTIONS") {
		t.Fatalf("system prompt missing phase skill: %q", sys)
	}
	for _, tool := range []string{"get_flow", "write_artifact", "advance_phase", "submit_verify", "archive"} {
		if !strings.Contains(sys, tool) {
			t.Fatalf("system prompt missing flow tool %s: %q", tool, sys)
		}
	}
}

func TestExecutorFlowWriteArtifactTool(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	flow := &fakeFlow{change: &domain.Change{ID: "c1", IssueID: "i1", Phase: "proposal", Status: "active"}}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"write_artifact","args":{"kind":"proposal","content":"# Proposal"}}`,
		`{"action":"final","message":"written"}`,
	}}
	e, _, _ := flowExecutor(t, client, flow, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(flow.writes) != 1 || flow.writes[0].kind != "proposal" || flow.writes[0].content != "# Proposal" || flow.writes[0].runID != "run-1" {
		t.Fatalf("writes = %+v", flow.writes)
	}
	// The tool result flows back into the next turn's transcript.
	if !strings.Contains(client.requests[1].user, "art-1") {
		t.Fatalf("second request missing artifact result: %q", client.requests[1].user)
	}
}

func TestExecutorFlowAdvancePhaseRefusalLogged(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	flow := &fakeFlow{
		change:     &domain.Change{ID: "c1", IssueID: "i1", Phase: "proposal", Status: "active"},
		advanceErr: fmt.Errorf("guard: phase advance refused: artifacts missing"),
	}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"advance_phase","args":{}}`,
		`{"action":"final","message":"gave up"}`,
	}}
	e, _, logs := flowExecutor(t, client, flow, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if flow.advanceCalled != 1 {
		t.Fatalf("advance called %d times", flow.advanceCalled)
	}
	// The refusal lands in the transcript so the model can correct course.
	if !strings.Contains(client.requests[1].user, "guard: phase advance refused") {
		t.Fatalf("refusal not fed back: %q", client.requests[1].user)
	}
	// ...and in the run log.
	var sawToolResult bool
	for _, l := range logs.logs {
		if l.Kind == "tool_result" && strings.Contains(l.Content, "guard: phase advance refused") {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Fatalf("refusal missing from run logs: %+v", logs.logs)
	}
}

func TestExecutorFlowGateBlocksTerminalStatusWhileActive(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T", Status: "in_progress"}
	flow := &fakeFlow{change: &domain.Change{ID: "c1", IssueID: "i1", Phase: "tasks", Status: "active"}}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"set_status","args":{"status":"in_review"}}`,
		`{"action":"tool","tool":"archive","args":{}}`,
		`{"action":"tool","tool":"set_status","args":{"status":"in_review"}}`,
		`{"action":"final","message":"all done"}`,
	}}
	e, issues, logs := flowExecutor(t, client, flow, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// First set_status was refused: issue unchanged.
	if len(issues.updated) != 1 || issues.updated[0].Status != "in_review" {
		t.Fatalf("issue updates = %+v, want exactly the post-archive one", issues.updated)
	}
	var gateLog bool
	for _, l := range logs.logs {
		if l.Kind == "error" && strings.Contains(l.Content, "classic flow") {
			gateLog = true
		}
	}
	if !gateLog {
		t.Fatalf("gate refusal not logged: %+v", logs.logs)
	}
	if flow.archiveCalled != 1 {
		t.Fatalf("archive called %d times", flow.archiveCalled)
	}
}

func TestExecutorFlowGetFlowTool(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	flow := &fakeFlow{
		change: &domain.Change{ID: "c1", IssueID: "i1", Phase: "design", Status: "active"},
		skill:  &skill.Skill{Key: "write-plan", Name: "Write Plan"},
	}
	client := &fakeLLM{responses: []string{
		`{"action":"tool","tool":"get_flow","args":{}}`,
		`{"action":"final","message":"ok"}`,
	}}
	e, _, _ := flowExecutor(t, client, flow, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(client.requests[1].user, "design") || !strings.Contains(client.requests[1].user, "write-plan") {
		t.Fatalf("get_flow result missing phase/skill: %q", client.requests[1].user)
	}
}

func TestExecutorFlowEnsureChangeFailureFailsRun(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}
	flow := &fakeFlow{} // change nil → EnsureChange errors
	client := &fakeLLM{responses: []string{`{"action":"final","message":"ok"}`}}
	e, _, _ := flowExecutor(t, client, flow, issue)
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	if err := e.Execute(context.Background(), run, agent); err == nil {
		t.Fatalf("EnsureChange failure should fail the run")
	}
}
