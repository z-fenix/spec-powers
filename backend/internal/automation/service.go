// Package automation implements inbound webhooks and autopilots: an inbound
// webhook receives external events (authenticated by HMAC signature) and
// fires the autopilots bound to it; autopilots run on a cron schedule, on a
// webhook event or manually, and either create an issue or enqueue an agent
// run.
package automation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"

	"specpowers/backend/internal/cronexpr"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/store"
)

// Trigger types.
const (
	TriggerCron    = "cron"
	TriggerWebhook = "webhook"
	TriggerManual  = "manual"
)

// Action types.
const (
	ActionCreateIssue = "create_issue"
	ActionRunAgent    = "run_agent"
)

// IssueCreator is the narrow issue-service surface the actions need.
type IssueCreator interface {
	CreateIssue(ctx context.Context, userID, projectID string, in issue.CreateInput) (*domain.Issue, error)
}

// RunCreator is the narrow run-store surface run_agent actions need.
type RunCreator interface {
	CreateRun(ctx context.Context, r *domain.Run) (*domain.Run, error)
}

// AutopilotInput carries the caller-provided fields for create/update.
type AutopilotInput struct {
	Name             string
	TriggerType      string
	CronSpec         string
	WebhookID        string
	ActionType       string
	AgentID          string
	ProjectID        string
	IssueID          string
	IssueTitle       string
	IssueDescription string
	Enabled          bool
}

// EventPayload is the (optional) JSON body of an inbound webhook event or a
// manual trigger. IssueID overrides the autopilot's target issue for
// run_agent actions; Title overrides the created issue's title for
// create_issue actions.
type EventPayload struct {
	IssueID string `json:"issue_id"`
	Title   string `json:"title"`
}

// Service manages webhooks and autopilots and executes their actions.
type Service struct {
	webhooks   store.WebhookStore
	autopilots store.AutopilotStore
	issues     IssueCreator
	runs       RunCreator
	now        func() time.Time
}

func NewService(webhooks store.WebhookStore, autopilots store.AutopilotStore, issues IssueCreator, runs RunCreator) *Service {
	return &Service{webhooks: webhooks, autopilots: autopilots, issues: issues, runs: runs, now: time.Now}
}

// WithNow overrides the clock (tests).
func (s *Service) WithNow(now func() time.Time) *Service {
	s.now = now
	return s
}

// CreateWebhook registers an inbound webhook endpoint and returns it with a
// freshly generated signing secret.
func (s *Service) CreateWebhook(ctx context.Context, name string) (*domain.Webhook, error) {
	if strings.TrimSpace(name) == "" {
		return nil, httpapi.ErrInvalid("webhook name is required")
	}
	w, err := s.webhooks.CreateWebhook(ctx, &domain.Webhook{
		Name:    name,
		Secret:  newSecret(),
		Enabled: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	return w, nil
}

func (s *Service) ListWebhooks(ctx context.Context) ([]domain.Webhook, error) {
	return s.webhooks.ListWebhooks(ctx)
}

func (s *Service) GetWebhook(ctx context.Context, id string) (*domain.Webhook, error) {
	w, err := s.webhooks.GetWebhook(ctx, id)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("webhook not found")
	}
	return w, err
}

// UpdateWebhook applies a partial update; nil fields are left unchanged.
func (s *Service) UpdateWebhook(ctx context.Context, id string, name *string, enabled *bool) (*domain.Webhook, error) {
	w, err := s.GetWebhook(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return nil, httpapi.ErrInvalid("webhook name is required")
		}
		w.Name = *name
	}
	if enabled != nil {
		w.Enabled = *enabled
	}
	updated, err := s.webhooks.UpdateWebhook(ctx, w)
	if err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	return updated, nil
}

func (s *Service) DeleteWebhook(ctx context.Context, id string) error {
	err := s.webhooks.DeleteWebhook(ctx, id)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("webhook not found")
	}
	return err
}

// CreateAutopilot validates and persists a new autopilot for the creating
// user; cron autopilots get their first next_run_at scheduled here.
func (s *Service) CreateAutopilot(ctx context.Context, userID string, in AutopilotInput) (*domain.Autopilot, error) {
	a := &domain.Autopilot{
		Name:             in.Name,
		TriggerType:      in.TriggerType,
		CronSpec:         in.CronSpec,
		WebhookID:        in.WebhookID,
		ActionType:       in.ActionType,
		AgentID:          in.AgentID,
		ProjectID:        in.ProjectID,
		IssueID:          in.IssueID,
		IssueTitle:       in.IssueTitle,
		IssueDescription: in.IssueDescription,
		CreatedBy:        userID,
		Enabled:          in.Enabled,
	}
	if err := s.validate(ctx, a); err != nil {
		return nil, err
	}
	if a.TriggerType == TriggerCron {
		next, err := s.nextCronRun(a.CronSpec)
		if err != nil {
			return nil, err
		}
		a.NextRunAt = &next
	}
	created, err := s.autopilots.CreateAutopilot(ctx, a)
	if err != nil {
		return nil, fmt.Errorf("create autopilot: %w", err)
	}
	return created, nil
}

func (s *Service) ListAutopilots(ctx context.Context) ([]domain.Autopilot, error) {
	return s.autopilots.ListAutopilots(ctx)
}

func (s *Service) GetAutopilot(ctx context.Context, id string) (*domain.Autopilot, error) {
	a, err := s.autopilots.GetAutopilot(ctx, id)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("autopilot not found")
	}
	return a, err
}

// UpdateAutopilot applies a full mutable update; when the trigger or cron
// spec changes, the next cron run is rescheduled.
func (s *Service) UpdateAutopilot(ctx context.Context, id string, in AutopilotInput) (*domain.Autopilot, error) {
	a, err := s.GetAutopilot(ctx, id)
	if err != nil {
		return nil, err
	}
	a.Name = in.Name
	a.TriggerType = in.TriggerType
	a.CronSpec = in.CronSpec
	a.WebhookID = in.WebhookID
	a.ActionType = in.ActionType
	a.AgentID = in.AgentID
	a.ProjectID = in.ProjectID
	a.IssueID = in.IssueID
	a.IssueTitle = in.IssueTitle
	a.IssueDescription = in.IssueDescription
	a.Enabled = in.Enabled
	if err := s.validate(ctx, a); err != nil {
		return nil, err
	}
	if a.TriggerType == TriggerCron {
		next, err := s.nextCronRun(a.CronSpec)
		if err != nil {
			return nil, err
		}
		a.NextRunAt = &next
	} else {
		a.NextRunAt = nil
	}
	updated, err := s.autopilots.UpdateAutopilot(ctx, a)
	if err != nil {
		return nil, fmt.Errorf("update autopilot: %w", err)
	}
	return updated, nil
}

func (s *Service) DeleteAutopilot(ctx context.Context, id string) error {
	err := s.autopilots.DeleteAutopilot(ctx, id)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("autopilot not found")
	}
	return err
}

// TriggerAutopilot fires one autopilot from a manual trigger or a validated
// inbound webhook event. Disabled autopilots refuse to fire.
func (s *Service) TriggerAutopilot(ctx context.Context, id string, payload EventPayload) error {
	a, err := s.GetAutopilot(ctx, id)
	if err != nil {
		return err
	}
	if !a.Enabled {
		return httpapi.ErrConflict("autopilot is disabled")
	}
	return s.fire(ctx, a, payload)
}

// fire executes the autopilot's action and stamps last_fired_at (plus the
// next cron run for cron triggers). Action failures surface to the caller.
func (s *Service) fire(ctx context.Context, a *domain.Autopilot, payload EventPayload) error {
	if err := s.executeAction(ctx, a, payload); err != nil {
		return err
	}
	now := s.now()
	a.LastFiredAt = &now
	if a.TriggerType == TriggerCron {
		if sched, err := cronexpr.Parse(a.CronSpec); err == nil {
			next := sched.Next(now)
			a.NextRunAt = &next
		}
	}
	if _, err := s.autopilots.UpdateAutopilot(ctx, a); err != nil {
		return fmt.Errorf("record autopilot fire: %w", err)
	}
	return nil
}

// executeAction runs the autopilot's configured action. The payload may
// override the run_agent target issue and the created issue's title.
func (s *Service) executeAction(ctx context.Context, a *domain.Autopilot, payload EventPayload) error {
	switch a.ActionType {
	case ActionCreateIssue:
		title := a.IssueTitle
		if payload.Title != "" {
			title = payload.Title
		}
		_, err := s.issues.CreateIssue(ctx, a.CreatedBy, a.ProjectID, issue.CreateInput{
			Title:       title,
			Description: a.IssueDescription,
		})
		return err
	case ActionRunAgent:
		issueID := a.IssueID
		if payload.IssueID != "" {
			issueID = payload.IssueID
		}
		_, err := s.runs.CreateRun(ctx, &domain.Run{
			AgentID: a.AgentID,
			IssueID: issueID,
			Trigger: "autopilot",
		})
		return err
	default:
		return httpapi.ErrInvalid("unknown action type: " + a.ActionType)
	}
}

// validate checks trigger/action configuration invariants shared by create
// and update.
func (s *Service) validate(ctx context.Context, a *domain.Autopilot) error {
	if strings.TrimSpace(a.Name) == "" {
		return httpapi.ErrInvalid("autopilot name is required")
	}
	switch a.TriggerType {
	case TriggerCron:
		if _, err := cronexpr.Parse(a.CronSpec); err != nil {
			return httpapi.ErrInvalid("invalid cron spec")
		}
	case TriggerWebhook:
		if a.WebhookID == "" {
			return httpapi.ErrInvalid("webhook trigger requires a webhook")
		}
		if _, err := s.webhooks.GetWebhook(ctx, a.WebhookID); err == store.ErrNotFound {
			return httpapi.ErrNotFound("webhook not found")
		}
	case TriggerManual:
	default:
		return httpapi.ErrInvalid("unknown trigger type: " + a.TriggerType)
	}
	switch a.ActionType {
	case ActionCreateIssue:
		if a.ProjectID == "" {
			return httpapi.ErrInvalid("create_issue action requires a project")
		}
		if strings.TrimSpace(a.IssueTitle) == "" {
			return httpapi.ErrInvalid("create_issue action requires an issue title")
		}
	case ActionRunAgent:
		if a.AgentID == "" || a.IssueID == "" {
			return httpapi.ErrInvalid("run_agent action requires an agent and an issue")
		}
	default:
		return httpapi.ErrInvalid("unknown action type: " + a.ActionType)
	}
	return nil
}

func (s *Service) nextCronRun(spec string) (time.Time, error) {
	sched, err := cronexpr.Parse(spec)
	if err != nil {
		return time.Time{}, httpapi.ErrInvalid("invalid cron spec")
	}
	return sched.Next(s.now()), nil
}

// newSecret returns a 256-bit URL-safe random signing secret.
func newSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Printf("automation: secret generation failed: %v", err)
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func trimEmpty(s string) bool { return len(s) == 0 || len(trimSpace(s)) == 0 }

func trimSpace(s string) string {
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// fireWebhook fires every enabled autopilot bound to the webhook and
// returns how many fired successfully; one failing autopilot does not stop
// the others.
func (s *Service) fireWebhook(ctx context.Context, webhookID string, payload EventPayload) (int, error) {
	list, err := s.autopilots.ListAutopilotsByWebhook(ctx, webhookID, true)
	if err != nil {
		return 0, err
	}
	fired := 0
	for i := range list {
		if err := s.fire(ctx, &list[i], payload); err != nil {
			log.Printf("automation: autopilot %s (%s) failed: %v", list[i].ID, list[i].Name, err)
			continue
		}
		fired++
	}
	return fired, nil
}
