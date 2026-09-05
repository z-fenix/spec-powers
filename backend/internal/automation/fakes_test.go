package automation

import (
	"context"
	"sync"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/store"
)

// fakeStores is an in-memory WebhookStore + AutopilotStore for tests.
type fakeStores struct {
	mu         sync.Mutex
	webhooks   map[string]domain.Webhook
	autopilots map[string]domain.Autopilot
	nextID     int
}

func newFakeStores() *fakeStores {
	return &fakeStores{
		webhooks:   map[string]domain.Webhook{},
		autopilots: map[string]domain.Autopilot{},
	}
}

func (f *fakeStores) id() string {
	f.nextID++
	return string(rune('a' + f.nextID - 1))
}

func (f *fakeStores) CreateWebhook(_ context.Context, w *domain.Webhook) (*domain.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.ID = f.id()
	w.CreatedAt = time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	f.webhooks[w.ID] = *w
	out := *w
	return &out, nil
}

func (f *fakeStores) GetWebhook(_ context.Context, id string) (*domain.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.webhooks[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &w, nil
}

func (f *fakeStores) ListWebhooks(_ context.Context) ([]domain.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []domain.Webhook
	for _, w := range f.webhooks {
		list = append(list, w)
	}
	return list, nil
}

func (f *fakeStores) UpdateWebhook(_ context.Context, w *domain.Webhook) (*domain.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.webhooks[w.ID]; !ok {
		return nil, store.ErrNotFound
	}
	f.webhooks[w.ID] = *w
	out := *w
	return &out, nil
}

func (f *fakeStores) DeleteWebhook(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.webhooks[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.webhooks, id)
	return nil
}

func (f *fakeStores) CreateAutopilot(_ context.Context, a *domain.Autopilot) (*domain.Autopilot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a.ID = f.id()
	a.CreatedAt = time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	f.autopilots[a.ID] = *a
	out := *a
	return &out, nil
}

func (f *fakeStores) GetAutopilot(_ context.Context, id string) (*domain.Autopilot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.autopilots[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &a, nil
}

func (f *fakeStores) ListAutopilots(_ context.Context) ([]domain.Autopilot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []domain.Autopilot
	for _, a := range f.autopilots {
		list = append(list, a)
	}
	return list, nil
}

func (f *fakeStores) UpdateAutopilot(_ context.Context, a *domain.Autopilot) (*domain.Autopilot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.autopilots[a.ID]; !ok {
		return nil, store.ErrNotFound
	}
	f.autopilots[a.ID] = *a
	out := *a
	return &out, nil
}

func (f *fakeStores) DeleteAutopilot(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.autopilots[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.autopilots, id)
	return nil
}

func (f *fakeStores) ListAutopilotsByWebhook(_ context.Context, webhookID string, enabledOnly bool) ([]domain.Autopilot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []domain.Autopilot
	for _, a := range f.autopilots {
		if a.WebhookID != webhookID {
			continue
		}
		if enabledOnly && !a.Enabled {
			continue
		}
		list = append(list, a)
	}
	return list, nil
}

func (f *fakeStores) DueCronAutopilots(_ context.Context, now time.Time) ([]domain.Autopilot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []domain.Autopilot
	for _, a := range f.autopilots {
		if a.TriggerType != TriggerCron || !a.Enabled {
			continue
		}
		if a.NextRunAt == nil || !a.NextRunAt.After(now) {
			list = append(list, a)
		}
	}
	return list, nil
}

// fakeIssues records CreateIssue calls.
type fakeIssues struct {
	calls []fakeIssueCall
	err   error
}

type fakeIssueCall struct {
	UserID    string
	ProjectID string
	Input     issue.CreateInput
}

func (f *fakeIssues) CreateIssue(_ context.Context, userID, projectID string, in issue.CreateInput) (*domain.Issue, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, fakeIssueCall{UserID: userID, ProjectID: projectID, Input: in})
	return &domain.Issue{ID: "new-issue", Title: in.Title}, nil
}

// fakeRuns records CreateRun calls.
type fakeRuns struct {
	runs []domain.Run
	err  error
}

func (f *fakeRuns) CreateRun(_ context.Context, r *domain.Run) (*domain.Run, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.runs = append(f.runs, *r)
	out := *r
	out.ID = "new-run"
	return &out, nil
}

// newTestService wires a Service over fakes with a fixed clock.
func newTestService() (*Service, *fakeStores, *fakeIssues, *fakeRuns) {
	f := newFakeStores()
	issues := &fakeIssues{}
	runs := &fakeRuns{}
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	svc := NewService(f, f, issues, runs).WithNow(func() time.Time { return now })
	return svc, f, issues, runs
}
