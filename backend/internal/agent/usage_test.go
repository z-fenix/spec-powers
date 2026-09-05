package agent

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// fakeUsageRecorder captures RecordRunUsage calls.
type fakeUsageRecorder struct {
	recorded []domain.RunUsage
	failNext error
}

func (f *fakeUsageRecorder) RecordRunUsage(_ context.Context, runID string, promptTokens, completionTokens int64) error {
	if f.failNext != nil {
		return f.failNext
	}
	f.recorded = append(f.recorded, domain.RunUsage{
		RunID: runID, PromptTokens: promptTokens, CompletionTokens: completionTokens,
	})
	return nil
}

func TestExecutorRecordsUsagePerTurn(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "Build it"}
	// Turn 1 is a tool call (read_issue) so the loop runs two completions.
	client := &fakeLLM{
		responses: []string{
			`{"action":"tool","tool":"read_issue","args":{}}`,
			`{"action":"final","message":"done"}`,
		},
		promptTokens: 10, completionTokens: 5,
	}
	e, _, _, _, _ := newExecutorForTest(t, client, issue)
	usage := &fakeUsageRecorder{}
	e.usage = usage

	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Each of the fakeLLM's completions reports 10 prompt / 5 completion
	// tokens; two turns must produce two per-completion records.
	if len(usage.recorded) != 2 {
		t.Fatalf("recorded = %d entries, want 2", len(usage.recorded))
	}
	for i, u := range usage.recorded {
		if u.RunID != "run-1" {
			t.Errorf("record %d run = %q, want run-1", i, u.RunID)
		}
		if u.PromptTokens != 10 || u.CompletionTokens != 5 {
			t.Errorf("record %d usage = (%d, %d), want (10, 5)", i, u.PromptTokens, u.CompletionTokens)
		}
	}
}

func TestExecutorUsageRecordingFailureIsNonFatal(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "Build it"}
	client := &fakeLLM{responses: []string{`{"action":"final","message":"done"}`}}
	e, _, _, _, logs := newExecutorForTest(t, client, issue)
	e.usage = &fakeUsageRecorder{failNext: store.ErrNotFound}

	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute should not fail when usage recording fails: %v", err)
	}
	if len(logs.logs) == 0 {
		t.Fatal("expected an error log about the failed usage recording")
	}
}

func TestExecutorSkipsUsageWhenZero(t *testing.T) {
	issue := &domain.Issue{ID: "i1", ProjectID: "p1", Title: "Build it"}
	// fakeLLM defaults to zero token counts; the recorder must see nothing.
	client := &fakeLLM{responses: []string{`{"action":"final","message":"done"}`}}
	e, _, _, _, _ := newExecutorForTest(t, client, issue)
	usage := &fakeUsageRecorder{}
	e.usage = usage

	run := &domain.Run{ID: "run-1", AgentID: "agent-1", IssueID: "i1"}
	agent := &domain.Agent{ID: "agent-1", Name: "A"}
	if err := e.Execute(context.Background(), run, agent); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(usage.recorded) != 0 {
		t.Fatalf("recorded = %d entries, want 0 for zero-token completions", len(usage.recorded))
	}
}
