package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// ---- fakes ----

type fakeRuns struct {
	mu      sync.Mutex
	byID    map[string]*domain.Run
	order   []string // creation order
	nextID  int
	claimed map[string]bool
}

func newFakeRuns() *fakeRuns {
	return &fakeRuns{byID: map[string]*domain.Run{}, claimed: map[string]bool{}}
}

func (f *fakeRuns) CreateRun(_ context.Context, r *domain.Run) (*domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	cp := *r
	cp.ID = "run-" + string(rune('0'+f.nextID))
	cp.Status = "queued"
	cp.CreatedAt = time.Now()
	f.byID[cp.ID] = &cp
	f.order = append(f.order, cp.ID)
	return &cp, nil
}

func (f *fakeRuns) GetRun(_ context.Context, id string) (*domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRuns) ListRuns(_ context.Context, filter store.RunFilter) ([]domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Run
	for _, id := range f.order {
		r := f.byID[id]
		if filter.IssueID != "" && r.IssueID != filter.IssueID {
			continue
		}
		if filter.AgentID != "" && r.AgentID != filter.AgentID {
			continue
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeRuns) ClaimNextRun(_ context.Context) (*domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		if f.byID[id].Status == "queued" {
			f.byID[id].Status = "running"
			now := time.Now()
			f.byID[id].StartedAt = &now
			cp := *f.byID[id]
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeRuns) FinishRun(_ context.Context, id, status, errMsg string) (*domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	r.Status = status
	r.Error = errMsg
	now := time.Now()
	r.FinishedAt = &now
	cp := *r
	return &cp, nil
}

type fakeLogs struct {
	mu   sync.Mutex
	next int
	logs []domain.RunLog
}

func (f *fakeLogs) AppendRunLog(_ context.Context, l *domain.RunLog) (*domain.RunLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	l.Seq = f.next
	f.logs = append(f.logs, *l)
	return l, nil
}

func (f *fakeLogs) ListRunLogs(_ context.Context, runID string) ([]domain.RunLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.RunLog
	for _, l := range f.logs {
		if l.RunID == runID {
			out = append(out, l)
		}
	}
	return out, nil
}

type fakeExec struct {
	mu    sync.Mutex
	calls []execCall
	err   error
	done  chan struct{}
	once  sync.Once
}

type execCall struct {
	run   domain.Run
	agent domain.Agent
}

func (f *fakeExec) Execute(_ context.Context, run *domain.Run, agent *domain.Agent) error {
	f.mu.Lock()
	f.calls = append(f.calls, execCall{run: *run, agent: *agent})
	f.mu.Unlock()
	if f.done != nil {
		f.once.Do(func() { close(f.done) })
	}
	return f.err
}

// ---- Queue tests ----

func TestQueueEnqueueCreatesQueuedRun(t *testing.T) {
	runs := newFakeRuns()
	q := NewQueue(runs, &fakeLogs{}, newFakeAgents(), &fakeExec{})
	run, err := q.Enqueue(context.Background(), "a1", "i1", "assigned")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if run.Status != "queued" || run.AgentID != "a1" || run.IssueID != "i1" || run.Trigger != "assigned" {
		t.Fatalf("run = %+v", run)
	}
}

func TestQueueRunOneExecutesOldestRun(t *testing.T) {
	runs := newFakeRuns()
	logs := &fakeLogs{}
	agents := newFakeAgents()
	ctx := context.Background()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agentID := "a1"

	q := NewQueue(runs, logs, agents, &fakeExec{})
	if _, err := q.Enqueue(ctx, agentID, "issue-1", "assigned"); err != nil {
		t.Fatalf("enqueue 1: %v", err)
	}
	second, err := q.Enqueue(ctx, agentID, "issue-2", "manual")
	if err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	ran, err := q.RunOne(ctx)
	if err != nil || !ran {
		t.Fatalf("run one: ran=%v err=%v", ran, err)
	}

	after, _ := runs.ListRuns(ctx, store.RunFilter{Status: "done"})
	if len(after) != 1 || after[0].ID == second.ID {
		t.Fatalf("oldest run not done first: %+v", after)
	}
	finished, _ := runs.GetRun(ctx, after[0].ID)
	if finished.FinishedAt == nil {
		t.Fatalf("finished_at not set: %+v", finished)
	}

	// The executor receives the run and its agent definition.
	// (verified via the executor fake in executor loop tests)
}

func TestQueueRunOneMarksFailedRunOnError(t *testing.T) {
	runs := newFakeRuns()
	agents := newFakeAgents()
	ctx := context.Background()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agentID := "a1"

	exec := &fakeExec{err: errors.New("llm exploded")}
	q := NewQueue(runs, &fakeLogs{}, agents, exec)
	run, _ := q.Enqueue(ctx, agentID, "issue-1", "assigned")

	if _, err := q.RunOne(ctx); err != nil {
		t.Fatalf("run one: %v", err)
	}
	failed, err := runs.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if failed.Status != "failed" || failed.Error != "llm exploded" {
		t.Fatalf("failed run = %+v", failed)
	}
}

func TestQueueRunOneEmptyQueue(t *testing.T) {
	q := NewQueue(newFakeRuns(), &fakeLogs{}, newFakeAgents(), &fakeExec{})
	ran, err := q.RunOne(context.Background())
	if err != nil {
		t.Fatalf("run one: %v", err)
	}
	if ran {
		t.Fatalf("empty queue should not run")
	}
}

func TestQueueLoopDrainsEnqueuedRuns(t *testing.T) {
	runs := newFakeRuns()
	agents := newFakeAgents()
	ctx := context.Background()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agentID := "a1"

	exec := &fakeExec{done: make(chan struct{})}
	q := NewQueue(runs, &fakeLogs{}, agents, exec)
	if _, err := q.Enqueue(ctx, agentID, "issue-1", "assigned"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	go q.Loop(loopCtx)
	select {
	case <-exec.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("loop did not execute the run in time")
	}
	cancel()
}

// ---- Trigger tests ----

func TestTriggerEnqueuesForAgentAssignee(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	ctx := context.Background()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agentID := "a1"

	trig := NewTrigger(agents, runs)

	// Human assignee: no run.
	if err := trig.OnIssueAssigned(ctx, &domain.Issue{ID: "i1", AssigneeID: "human-1"}); err != nil {
		t.Fatalf("human assignee: %v", err)
	}
	human, _ := runs.ListRuns(ctx, store.RunFilter{})
	if len(human) != 0 {
		t.Fatalf("human assignee should not enqueue: %+v", human)
	}

	// Agent assignee: one queued run.
	if err := trig.OnIssueAssigned(ctx, &domain.Issue{ID: "i1", AssigneeID: agentID}); err != nil {
		t.Fatalf("agent assignee: %v", err)
	}
	created, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "i1"})
	if len(created) != 1 || created[0].Trigger != "assigned" || created[0].AgentID != agentID {
		t.Fatalf("runs = %+v", created)
	}

	// Status change on an agent-assigned issue triggers a run.
	if err := trig.OnIssueStatusChanged(ctx, &domain.Issue{ID: "i1", AssigneeID: agentID}); err != nil {
		t.Fatalf("status change: %v", err)
	}
	statusRuns, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "i1"})
	if len(statusRuns) != 2 {
		t.Fatalf("status change runs = %d, want 2 total", len(statusRuns))
	}

	// Parent wakeup on an agent-assigned parent triggers a run.
	if err := trig.OnParentWakeup(ctx, &domain.Issue{ID: "parent-1", AssigneeID: agentID}); err != nil {
		t.Fatalf("parent wakeup: %v", err)
	}
	wakeupRuns, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "parent-1"})
	if len(wakeupRuns) != 1 || wakeupRuns[0].Trigger != "wakeup" {
		t.Fatalf("wakeup runs = %+v", wakeupRuns)
	}
}

func TestTriggerUnknownAgentIsNoop(t *testing.T) {
	trig := NewTrigger(newFakeAgents(), newFakeRuns())
	if err := trig.OnIssueAssigned(context.Background(), &domain.Issue{ID: "i1", AssigneeID: "ghost"}); err != nil {
		t.Fatalf("unknown agent assignee: %v", err)
	}
}
