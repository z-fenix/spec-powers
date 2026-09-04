package agent

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

func seedMentionAgent(t *testing.T, agents *fakeAgents, id, name string) {
	t.Helper()
	if _, err := agents.CreateAgent(context.Background(), &domain.Agent{ID: id, Name: name}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
}

func TestMentionTriggerEnqueuesRunForMentionedAgent(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	seedMentionAgent(t, agents, "a1", "KunCoding")
	trig := NewMentionTrigger(agents, runs)
	ctx := context.Background()

	if err := trig.OnComment(ctx, "i1", "author-1", "hey @KunCoding please take this"); err != nil {
		t.Fatalf("on comment: %v", err)
	}
	created, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "i1"})
	if len(created) != 1 || created[0].AgentID != "a1" || created[0].Trigger != "mention" || created[0].Status != "queued" {
		t.Fatalf("runs = %+v", created)
	}
}

func TestMentionTriggerIgnoresNonAgentsAndSelf(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	seedMentionAgent(t, agents, "a1", "KunCoding")
	trig := NewMentionTrigger(agents, runs)
	ctx := context.Background()

	// Unknown @name: nothing.
	if err := trig.OnComment(ctx, "i1", "author-1", "@GhostHandler help"); err != nil {
		t.Fatalf("unknown mention: %v", err)
	}
	// Plain name without @: nothing.
	if err := trig.OnComment(ctx, "i1", "author-1", "KunCoding looks great"); err != nil {
		t.Fatalf("plain name: %v", err)
	}
	// Self-mention (author is the agent): skipped, no self-loop.
	if err := trig.OnComment(ctx, "i1", "a1", "@KunCoding noted"); err != nil {
		t.Fatalf("self mention: %v", err)
	}
	// Case-insensitive match still triggers.
	if err := trig.OnComment(ctx, "i1", "author-1", "@kuncoding run"); err != nil {
		t.Fatalf("case mention: %v", err)
	}
	created, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "i1"})
	if len(created) != 1 {
		t.Fatalf("runs = %+v, want exactly 1 (case-insensitive)", created)
	}
}

func TestMentionTriggerMultipleAgents(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	seedMentionAgent(t, agents, "a1", "KunCoding")
	seedMentionAgent(t, agents, "a2", "Reviewer")
	trig := NewMentionTrigger(agents, runs)
	ctx := context.Background()

	if err := trig.OnComment(ctx, "i1", "author-1", "@KunCoding build it, @Reviewer check after"); err != nil {
		t.Fatalf("on comment: %v", err)
	}
	created, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "i1"})
	if len(created) != 2 {
		t.Fatalf("runs = %+v, want 2", created)
	}
	seen := map[string]bool{}
	for _, r := range created {
		seen[r.AgentID] = true
	}
	if !seen["a1"] || !seen["a2"] {
		t.Fatalf("missing agents: %+v", seen)
	}
}
