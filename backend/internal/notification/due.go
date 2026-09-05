package notification

import (
	"context"
	"log"
	"time"

	"specpowers/backend/internal/domain"
)

// DueSoonWindow is how close to a due date a "due soon" notification fires.
const DueSoonWindow = 24 * time.Hour

// DueIssueSource lists the issues whose deadlines the scanner watches.
type DueIssueSource interface {
	// ListIssuesWithDueDate returns every open issue that has a due date,
	// across all projects.
	ListIssuesWithDueDate(ctx context.Context) ([]domain.Issue, error)
}

// AgentLookup distinguishes agent assignees (runs are their notification
// channel) from human assignees.
type AgentLookup interface {
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
}

// DedupeStore answers whether an identical notification already exists so
// repeated scans stay idempotent.
type DedupeStore interface {
	HasNotificationForIssue(ctx context.Context, userID, issueID, kind, title string) (bool, error)
}

// DueScanner writes "due" notifications for a human assignee when an issue's
// deadline is approaching (within DueSoonWindow) or has passed. Each state
// notifies once per issue: repeated scans are deduped on (user, issue,
// title) so the due-soon and overdue notices can each fire exactly once.
type DueScanner struct {
	issues DueIssueSource
	agents AgentLookup
	dedupe DedupeStore
	sink   Sink
	now    func() time.Time
	window time.Duration
}

func NewDueScanner(issues DueIssueSource, agents AgentLookup, dedupe DedupeStore, sink Sink) *DueScanner {
	return &DueScanner{
		issues: issues,
		agents: agents,
		dedupe: dedupe,
		sink:   sink,
		now:    time.Now,
		window: DueSoonWindow,
	}
}

// WithNow overrides the clock (tests).
func (s *DueScanner) WithNow(now func() time.Time) *DueScanner {
	s.now = now
	return s
}

// WithWindow overrides the due-soon window (tests).
func (s *DueScanner) WithWindow(d time.Duration) *DueScanner {
	s.window = d
	return s
}

// Scan checks every due issue once and returns the first source error.
func (s *DueScanner) Scan(ctx context.Context) error {
	list, err := s.issues.ListIssuesWithDueDate(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for i := range list {
		s.notifyIssue(ctx, &list[i], now)
	}
	return nil
}

func (s *DueScanner) notifyIssue(ctx context.Context, i *domain.Issue, now time.Time) {
	if s.sink == nil || s.dedupe == nil || i.AssigneeID == "" || i.DueDate == nil {
		return
	}
	if s.isAgent(ctx, i.AssigneeID) {
		return
	}
	var title string
	due := *i.DueDate
	switch {
	case due.Before(now):
		title = "Issue overdue: " + i.Title
	case due.Sub(now) <= s.window:
		title = "Issue due soon: " + i.Title
	default:
		return
	}
	exists, err := s.dedupe.HasNotificationForIssue(ctx, i.AssigneeID, i.ID, "due", title)
	if err != nil {
		log.Printf("notification: due dedupe check failed: %v", err)
		return
	}
	if exists {
		return
	}
	s.sink.Notify(ctx, NotifyInput{
		UserID:    i.AssigneeID,
		Kind:      "due",
		Title:     title,
		IssueID:   i.ID,
		ProjectID: i.ProjectID,
	})
}

func (s *DueScanner) isAgent(ctx context.Context, id string) bool {
	if s.agents == nil {
		return false
	}
	_, err := s.agents.GetAgent(ctx, id)
	return err == nil
}

// Loop runs Scan on a ticker until ctx is cancelled; scan errors are logged,
// never fatal — the next tick retries.
func (s *DueScanner) Loop(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Scan(ctx); err != nil {
				log.Printf("notification: due scan failed: %v", err)
			}
		}
	}
}
