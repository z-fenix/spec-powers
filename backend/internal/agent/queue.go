package agent

import (
	"context"
	"errors"
	"log"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/notification"
	"specpowers/backend/internal/store"
)

// RunExecutor performs one claimed run. Implementations receive the run and
// its agent definition and return an error to mark the run failed.
type RunExecutor interface {
	Execute(ctx context.Context, run *domain.Run, agent *domain.Agent) error
}

// Queue is the persisted run queue: enqueue creates queued rows, the worker
// claims them FIFO and drives the executor through the lifecycle
// queued → running → done | failed.
type Queue struct {
	runs      store.RunStore
	logs      store.RunLogStore
	agents    store.AgentStore
	exec      RunExecutor
	poll      time.Duration
	notifier  notification.Sink
	notIssues issueAssigneeLookup
}

// issueAssigneeLookup lets the queue notify the issue's assignee when a run
// finishes.
type issueAssigneeLookup interface {
	GetIssue(ctx context.Context, id string) (*domain.Issue, error)
}

func NewQueue(runs store.RunStore, logs store.RunLogStore, agents store.AgentStore, exec RunExecutor) *Queue {
	return &Queue{runs: runs, logs: logs, agents: agents, exec: exec, poll: time.Second}
}

// WithNotifier attaches a notification sink and the issue lookup used to
// resolve assignees; finished runs then notify the issue's assignee.
func (q *Queue) WithNotifier(n notification.Sink, issues issueAssigneeLookup) *Queue {
	q.notifier = n
	q.notIssues = issues
	return q
}

// notifyRunFinished tells the issue's assignee that a run reached its final
// state; issues without an assignee stay silent.
func (q *Queue) notifyRunFinished(ctx context.Context, run *domain.Run, status, errMsg string) {
	if q.notifier == nil || q.notIssues == nil {
		return
	}
	i, err := q.notIssues.GetIssue(ctx, run.IssueID)
	if err != nil || i.AssigneeID == "" {
		return
	}
	statusLabel := "finished"
	if status == "failed" {
		statusLabel = "failed"
	}
	q.notifier.Notify(ctx, notification.NotifyInput{
		UserID:    i.AssigneeID,
		Kind:      "run_finished",
		Title:     "Agent run " + statusLabel + " on: " + i.Title,
		Body:      errMsg,
		IssueID:   i.ID,
		ProjectID: i.ProjectID,
	})
}

// WithPoll overrides the worker polling interval (tests).
func (q *Queue) WithPoll(d time.Duration) *Queue {
	q.poll = d
	return q
}

// Enqueue creates a queued run.
func (q *Queue) Enqueue(ctx context.Context, agentID, issueID, trigger string) (*domain.Run, error) {
	run, err := q.runs.CreateRun(ctx, &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: trigger})
	if err != nil {
		return nil, httpapi.ErrInternal("enqueue run failed")
	}
	return run, nil
}

// RunOne claims and executes the oldest queued run. It reports whether a run
// was executed; an empty queue is not an error.
func (q *Queue) RunOne(ctx context.Context) (bool, error) {
	run, err := q.runs.ClaimNextRun(ctx)
	if err == store.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := q.execute(ctx, run); err != nil {
		if _, ferr := q.runs.FinishRun(ctx, run.ID, "failed", err.Error()); ferr != nil {
			return true, ferr
		}
		q.notifyRunFinished(ctx, run, "failed", err.Error())
		return true, nil
	}
	if _, ferr := q.runs.FinishRun(ctx, run.ID, "done", ""); ferr != nil {
		return true, ferr
	}
	q.notifyRunFinished(ctx, run, "done", "")
	return true, nil
}

func (q *Queue) execute(ctx context.Context, run *domain.Run) error {
	a, err := q.agents.GetAgent(ctx, run.AgentID)
	if err != nil {
		if err == store.ErrNotFound {
			return errors.New("agent not found: " + run.AgentID)
		}
		return err
	}
	return q.exec.Execute(ctx, run, a)
}

// Loop polls the queue until ctx is cancelled. Claim/execute errors are
// logged, never panic the loop.
func (q *Queue) Loop(ctx context.Context) {
	ticker := time.NewTicker(q.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				ran, err := q.RunOne(ctx)
				if err != nil {
					log.Printf("agent queue: run failed: %v", err)
					break
				}
				if !ran {
					break
				}
			}
		}
	}
}

// Trigger implements issue.RunTrigger: it enqueues a run whenever an issue
// is assigned to an agent, its status changes while agent-assigned, or an
// agent-assigned parent is woken by its children reaching terminal states.
type Trigger struct {
	agents store.AgentStore
	runs   store.RunStore
}

var _ issue.RunTrigger = (*Trigger)(nil)

func NewTrigger(agents store.AgentStore, runs store.RunStore) *Trigger {
	return &Trigger{agents: agents, runs: runs}
}

func (t *Trigger) isAgent(ctx context.Context, id string) bool {
	if id == "" {
		return false
	}
	_, err := t.agents.GetAgent(ctx, id)
	return err == nil
}

func (t *Trigger) enqueue(ctx context.Context, issueID, agentID, trigger string) error {
	_, err := t.runs.CreateRun(ctx, &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: trigger})
	return err
}

func (t *Trigger) OnIssueAssigned(ctx context.Context, i *domain.Issue) error {
	if !t.isAgent(ctx, i.AssigneeID) {
		return nil
	}
	return t.enqueue(ctx, i.ID, i.AssigneeID, "assigned")
}

func (t *Trigger) OnIssueStatusChanged(ctx context.Context, i *domain.Issue) error {
	if !t.isAgent(ctx, i.AssigneeID) {
		return nil
	}
	return t.enqueue(ctx, i.ID, i.AssigneeID, "status_changed")
}

func (t *Trigger) OnParentWakeup(ctx context.Context, parent *domain.Issue) error {
	if !t.isAgent(ctx, parent.AssigneeID) {
		return nil
	}
	return t.enqueue(ctx, parent.ID, parent.AssigneeID, "wakeup")
}
