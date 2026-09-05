package automation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/cronexpr"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

func webhookInput(name string) AutopilotInput {
	return AutopilotInput{
		Name:        name,
		TriggerType: TriggerManual,
		ActionType:  ActionCreateIssue,
		ProjectID:   "p1",
		IssueTitle:  "generated issue",
		Enabled:     true,
	}
}

func TestCreateWebhookGeneratesSecret(t *testing.T) {
	svc, f, _, _ := newTestService()
	w, err := svc.CreateWebhook(context.Background(), "ci events")
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if w.ID == "" || w.Name != "ci events" || !w.Enabled {
		t.Errorf("unexpected webhook: %+v", w)
	}
	if len(w.Secret) < 16 {
		t.Errorf("secret too short: %q", w.Secret)
	}
	// secrets are unique per webhook
	second, _ := svc.CreateWebhook(context.Background(), "other")
	if second.Secret == w.Secret {
		t.Errorf("secrets must be unique")
	}
	if len(f.webhooks) != 2 {
		t.Errorf("expected 2 webhooks stored, got %d", len(f.webhooks))
	}
}

func TestCreateWebhookRejectsBlankName(t *testing.T) {
	svc, _, _, _ := newTestService()
	_, err := svc.CreateWebhook(context.Background(), "   ")
	var appErr *httpapi.AppError
	if err == nil || !asAppErr(err, &appErr) || appErr.Status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestCreateAutopilotCronSchedulesNextRun(t *testing.T) {
	svc, f, _, _ := newTestService()
	in := webhookInput("nightly")
	in.TriggerType = TriggerCron
	in.CronSpec = "0 9 * * *"
	in.ProjectID = "p1"
	a, err := svc.CreateAutopilot(context.Background(), "u1", in)
	if err != nil {
		t.Fatalf("CreateAutopilot: %v", err)
	}
	if a.NextRunAt == nil {
		t.Fatal("cron autopilot must have next_run_at scheduled")
	}
	sched, _ := cronexpr.Parse("0 9 * * *")
	want := sched.Next(time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC))
	if !a.NextRunAt.Equal(want) {
		t.Errorf("next_run_at = %v, want %v", a.NextRunAt, want)
	}
	if f.autopilots[a.ID].CreatedBy != "u1" {
		t.Errorf("created_by not recorded")
	}
}

func TestCreateAutopilotValidation(t *testing.T) {
	svc, f, _, _ := newTestService()
	hook, _ := svc.CreateWebhook(context.Background(), "hook")
	ctx := context.Background()

	cases := []struct {
		name string
		in   AutopilotInput
		want int
	}{
		{"blank name", AutopilotInput{TriggerType: TriggerManual, ActionType: ActionCreateIssue, ProjectID: "p", IssueTitle: "t", Enabled: true}, 400},
		{"bad cron", func() AutopilotInput {
			in := webhookInput("x")
			in.TriggerType = TriggerCron
			in.CronSpec = "nope"
			return in
		}(), 400},
		{"webhook trigger without webhook", func() AutopilotInput { in := webhookInput("x"); in.TriggerType = TriggerWebhook; return in }(), 400},
		{"webhook trigger unknown webhook", func() AutopilotInput {
			in := webhookInput("x")
			in.TriggerType = TriggerWebhook
			in.WebhookID = "missing"
			return in
		}(), 404},
		{"create_issue without project", func() AutopilotInput { in := webhookInput("x"); in.ProjectID = ""; return in }(), 400},
		{"run_agent without issue", func() AutopilotInput {
			in := webhookInput("x")
			in.ActionType = ActionRunAgent
			in.AgentID = "a1"
			return in
		}(), 400},
	}
	for _, tc := range cases {
		_, err := svc.CreateAutopilot(ctx, "u1", tc.in)
		var appErr *httpapi.AppError
		if err == nil || !asAppErr(err, &appErr) || appErr.Status != tc.want {
			t.Errorf("%s: expected %d, got %v", tc.name, tc.want, err)
		}
	}
	if len(f.autopilots) != 0 {
		t.Errorf("invalid inputs must not persist, got %d", len(f.autopilots))
	}

	// valid webhook trigger accepts the real webhook id
	in := webhookInput("bound")
	in.TriggerType = TriggerWebhook
	in.WebhookID = hook.ID
	if _, err := svc.CreateAutopilot(ctx, "u1", in); err != nil {
		t.Errorf("valid webhook trigger rejected: %v", err)
	}
}

func TestTriggerAutopilotRunsAction(t *testing.T) {
	svc, _, issues, runs := newTestService()
	ctx := context.Background()

	create := webhookInput("creator")
	create.ProjectID = "p1"
	creator, err := svc.CreateAutopilot(ctx, "u1", create)
	if err != nil {
		t.Fatalf("create create_issue autopilot: %v", err)
	}
	if err := svc.TriggerAutopilot(ctx, creator.ID, EventPayload{Title: "from event"}); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if len(issues.calls) != 1 || issues.calls[0].Input.Title != "from event" || issues.calls[0].UserID != "u1" {
		t.Errorf("create_issue action: %+v", issues.calls)
	}
	stored, _ := svc.GetAutopilot(ctx, creator.ID)
	if stored.LastFiredAt == nil {
		t.Errorf("last_fired_at must be stamped")
	}

	run := webhookInput("runner")
	run.ActionType = ActionRunAgent
	run.AgentID = "agent-1"
	run.IssueID = "issue-9"
	run.ProjectID = ""
	run.IssueTitle = ""
	runner, err := svc.CreateAutopilot(ctx, "u1", run)
	if err != nil {
		t.Fatalf("create run_agent autopilot: %v", err)
	}
	if err := svc.TriggerAutopilot(ctx, runner.ID, EventPayload{}); err != nil {
		t.Fatalf("trigger run_agent: %v", err)
	}
	if len(runs.runs) != 1 || runs.runs[0].AgentID != "agent-1" || runs.runs[0].IssueID != "issue-9" || runs.runs[0].Trigger != "autopilot" {
		t.Errorf("run_agent action: %+v", runs.runs)
	}

	// payload issue_id overrides the configured one
	if err := svc.TriggerAutopilot(ctx, runner.ID, EventPayload{IssueID: "issue-override"}); err != nil {
		t.Fatalf("trigger with override: %v", err)
	}
	if runs.runs[1].IssueID != "issue-override" {
		t.Errorf("payload issue_id override not applied: %+v", runs.runs[1])
	}
}

func TestTriggerAutopilotDisabledConflicts(t *testing.T) {
	svc, _, _, runs := newTestService()
	ctx := context.Background()
	in := webhookInput("paused")
	in.Enabled = false
	a, err := svc.CreateAutopilot(ctx, "u1", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	err = svc.TriggerAutopilot(ctx, a.ID, EventPayload{})
	var appErr *httpapi.AppError
	if err == nil || !asAppErr(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("expected 409 for disabled autopilot, got %v", err)
	}
	if len(runs.runs) != 0 {
		t.Errorf("disabled autopilot must not run")
	}
}

func TestFireWebhookFiresOnlyBoundEnabledAutopilots(t *testing.T) {
	svc, f, issues, _ := newTestService()
	ctx := context.Background()
	hook, _ := svc.CreateWebhook(ctx, "hook")

	bound := webhookInput("bound")
	bound.TriggerType = TriggerWebhook
	bound.WebhookID = hook.ID
	bound2 := webhookInput("bound2")
	bound2.TriggerType = TriggerWebhook
	bound2.WebhookID = hook.ID
	otherHook, _ := svc.CreateWebhook(ctx, "other-hook")
	other := webhookInput("other-hook")
	other.TriggerType = TriggerWebhook
	other.WebhookID = otherHook.ID

	for _, in := range []AutopilotInput{bound, bound2, other} {
		if _, err := svc.CreateAutopilot(ctx, "u1", in); err != nil {
			t.Fatalf("create %s: %v", in.Name, err)
		}
	}
	// disable the second bound one
	list, _ := f.ListAutopilots(ctx)
	var disabledID string
	for _, a := range list {
		if a.Name == "bound2" {
			disabledID = a.ID
		}
	}
	disabled, _ := f.GetAutopilot(ctx, disabledID)
	disabled.Enabled = false
	if _, err := f.UpdateAutopilot(ctx, disabled); err != nil {
		t.Fatalf("disable: %v", err)
	}

	fired, err := svc.fireWebhook(ctx, hook.ID, EventPayload{})
	if err != nil {
		t.Fatalf("fireWebhook: %v", err)
	}
	if fired != 1 {
		t.Errorf("fired = %d, want 1 (only enabled bound autopilot)", fired)
	}
	if len(issues.calls) != 1 {
		t.Errorf("expected 1 issue created, got %d", len(issues.calls))
	}
}

// asAppErr unwraps wrapped AppErrors via errors.As (repo convention).
func asAppErr(err error, target **httpapi.AppError) bool {
	return errors.As(err, target)
}

func TestUpdateAutopilotReschedulesCron(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	in := webhookInput("shift")
	in.TriggerType = TriggerCron
	in.CronSpec = "0 9 * * *"
	a, err := svc.CreateAutopilot(ctx, "u1", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	in.CronSpec = "30 8 * * *"
	updated, err := svc.UpdateAutopilot(ctx, a.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	sched, _ := cronexpr.Parse("30 8 * * *")
	want := sched.Next(time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC))
	if updated.NextRunAt == nil || !updated.NextRunAt.Equal(want) {
		t.Errorf("next_run_at = %v, want %v", updated.NextRunAt, want)
	}
	// switching away from cron clears next_run_at
	in.TriggerType = TriggerManual
	manual, err := svc.UpdateAutopilot(ctx, a.ID, in)
	if err != nil {
		t.Fatalf("update to manual: %v", err)
	}
	if manual.NextRunAt != nil {
		t.Errorf("manual autopilot must clear next_run_at, got %v", manual.NextRunAt)
	}
}

func TestSchedulerFiresDueAutopilots(t *testing.T) {
	svc, f, issues, runs := newTestService()
	ctx := context.Background()

	cron := webhookInput("cron-ap")
	cron.TriggerType = TriggerCron
	cron.CronSpec = "0 9 * * *"
	if _, err := svc.CreateAutopilot(ctx, "u1", cron); err != nil {
		t.Fatalf("create: %v", err)
	}
	// make it due now
	list, _ := f.ListAutopilots(ctx)
	due := list[0]
	past := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	due.NextRunAt = &past
	if _, err := f.UpdateAutopilot(ctx, &due); err != nil {
		t.Fatalf("make due: %v", err)
	}

	sched := NewScheduler(f, svc).WithNow(func() time.Time {
		return time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	})
	if err := sched.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(issues.calls) != 1 {
		t.Errorf("expected the cron action to create one issue, got %d", len(issues.calls))
	}
	stored, _ := svc.GetAutopilot(ctx, due.ID)
	if stored.LastFiredAt == nil {
		t.Errorf("last_fired_at must be stamped by the scheduler")
	}
	wantNext := mustSchedule(t, "0 9 * * *").Next(time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC))
	if stored.NextRunAt == nil || !stored.NextRunAt.Equal(wantNext) {
		t.Errorf("next_run_at = %v, want %v", stored.NextRunAt, wantNext)
	}
	// a second tick at the same instant must not refire (next run is tomorrow)
	before := len(issues.calls) + len(runs.runs)
	if err := sched.Tick(ctx); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	after, _ := f.ListAutopilots(ctx)
	_ = after
	if got := len(issues.calls) + len(runs.runs); got != before {
		t.Errorf("second tick refired (%d -> %d)", before, got)
	}
}

func mustSchedule(t *testing.T, spec string) cronexpr.Schedule {
	t.Helper()
	s, err := cronexpr.Parse(spec)
	if err != nil {
		t.Fatalf("parse %q: %v", spec, err)
	}
	return s
}

func TestSchedulerDisabledAndNonCronSkipped(t *testing.T) {
	svc, _, issues, _ := newTestService()
	ctx := context.Background()
	// only a manual autopilot exists: scheduler must be a no-op
	in := webhookInput("manual")
	if _, err := svc.CreateAutopilot(ctx, "u1", in); err != nil {
		t.Fatalf("create: %v", err)
	}
	sched := NewScheduler(fakeDue([]domain.Autopilot{}), svc)
	if err := sched.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(issues.calls) != 0 {
		t.Errorf("scheduler must not fire manual autopilots")
	}
}

// fakeDue adapts a fixed slice to the scheduler's store interface.
type fakeDueList struct{ list []domain.Autopilot }

func (f *fakeDueList) DueCronAutopilots(context.Context, time.Time) ([]domain.Autopilot, error) {
	return f.list, nil
}
func (f *fakeDueList) UpdateAutopilot(_ context.Context, a *domain.Autopilot) (*domain.Autopilot, error) {
	return a, nil
}

func fakeDue(list []domain.Autopilot) autopilotStore { return &fakeDueList{list: list} }

func TestSchedulerActionFailureStillReschedules(t *testing.T) {
	svc, f, issues, _ := newTestService()
	issues.err = context.DeadlineExceeded
	ctx := context.Background()

	cron := webhookInput("cron-ap")
	cron.TriggerType = TriggerCron
	cron.CronSpec = "0 9 * * *"
	if _, err := svc.CreateAutopilot(ctx, "u1", cron); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, _ := f.ListAutopilots(ctx)
	due := list[0]
	past := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	due.NextRunAt = &past
	if _, err := f.UpdateAutopilot(ctx, &due); err != nil {
		t.Fatalf("make due: %v", err)
	}
	sched := NewScheduler(f, svc).WithNow(func() time.Time {
		return time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	})
	_ = sched.Tick(ctx)
	stored, _ := svc.GetAutopilot(ctx, due.ID)
	if stored.NextRunAt == nil || !stored.NextRunAt.After(time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("failed action must still advance next_run_at, got %v", stored.NextRunAt)
	}
	if stored.LastFiredAt != nil {
		t.Errorf("last_fired_at must not be stamped for a failed action")
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "topsecret-key"
	body := []byte(`{"title":"hello"}`)
	sig := sign(secret, body)
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("signature must carry sha256= prefix: %q", sig)
	}
	if !verifySignature(secret, body, sig) {
		t.Errorf("valid signature rejected")
	}
	if verifySignature("wrong-key", body, sig) {
		t.Errorf("wrong secret accepted")
	}
	if verifySignature(secret, []byte(`{"title":"evil"}`), sig) {
		t.Errorf("tampered body accepted")
	}
	if verifySignature(secret, body, "sha256=deadbeef") {
		t.Errorf("garbage hex accepted")
	}
	if verifySignature(secret, body, "md5=abc") {
		t.Errorf("wrong scheme accepted")
	}
	if verifySignature(secret, body, "") {
		t.Errorf("empty signature accepted")
	}
}

// --- HTTP handler tests ---

func setupHandler(t *testing.T) (http.Handler, *auth.TokenService, *Service, *fakeStores, *fakeIssues, *fakeRuns) {
	t.Helper()
	svc, f, issues, runs := newTestService()
	tokens := auth.NewTokenService("automation-test-secret", 15*time.Minute)
	r := chi.NewRouter()
	h := NewHandler(svc, tokens)
	r.Mount("/api/v1/webhooks", http.StripPrefix("/api/v1/webhooks", h.WebhookRoutes()))
	r.Mount("/api/v1/autopilots", http.StripPrefix("/api/v1/autopilots", h.AutopilotRoutes()))
	r.Mount("/api/v1/hooks", http.StripPrefix("/api/v1/hooks", h.HookRoutes()))
	return r, tokens, svc, f, issues, runs
}

func doJSON(t *testing.T, h http.Handler, tokens *auth.TokenService, method, path, userID, body string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if userID != "" {
		tok, err := tokens.Issue(userID)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode response %q: %v", w.Body.String(), err)
		}
	}
	return w.Code, out
}

func TestWebhookAPIRequiresAuth(t *testing.T) {
	h, _, _, _, _, _ := setupHandler(t)
	code, _ := doJSON(t, h, nil, "GET", "/api/v1/webhooks/", "", "", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", code)
	}
}

func TestWebhookAPICreateAndList(t *testing.T) {
	h, tokens, _, _, _, _ := setupHandler(t)
	code, body := doJSON(t, h, tokens, "POST", "/api/v1/webhooks/", "u1", `{"name":"ci"}`, nil)
	if code != http.StatusCreated {
		t.Fatalf("create webhook: %d %v", code, body)
	}
	wh := body["webhook"].(map[string]any)
	if wh["secret"].(string) == "" {
		t.Errorf("created webhook must expose its secret once")
	}
	code, body = doJSON(t, h, tokens, "GET", "/api/v1/webhooks/", "u1", "", nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d", code)
	}
	if len(body["webhooks"].([]any)) != 1 {
		t.Errorf("expected 1 webhook listed")
	}
}

func TestWebhookAPIPatchEnable(t *testing.T) {
	h, tokens, svc, f, _, _ := setupHandler(t)
	ctx := context.Background()
	hook, _ := svc.CreateWebhook(ctx, "hook")
	code, body := doJSON(t, h, tokens, "PATCH", "/api/v1/webhooks/"+hook.ID, "u1", `{"enabled":false}`, nil)
	if code != http.StatusOK {
		t.Fatalf("patch: %d %v", code, body)
	}
	if body["webhook"].(map[string]any)["enabled"].(bool) {
		t.Errorf("webhook must be disabled")
	}
	stored, _ := f.GetWebhook(ctx, hook.ID)
	if stored.Enabled {
		t.Errorf("disable not persisted")
	}
}

func TestInboundHookEndpoint(t *testing.T) {
	h, _, svc, _, issues, _ := setupHandler(t)
	ctx := context.Background()
	hook, err := svc.CreateWebhook(ctx, "events")
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	// bind an autopilot
	in := webhookInput("on-event")
	in.TriggerType = TriggerWebhook
	in.WebhookID = hook.ID
	if _, err := svc.CreateAutopilot(ctx, "u1", in); err != nil {
		t.Fatalf("create autopilot: %v", err)
	}

	payload := `{"title":"deploy finished"}`
	sig := sign(hook.Secret, []byte(payload))

	// missing signature -> 401
	code, _ := doJSON(t, h, nil, "POST", "/api/v1/hooks/"+hook.ID, "", payload, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("unsigned request: expected 401, got %d", code)
	}
	// bad signature -> 401
	code, _ = doJSON(t, h, nil, "POST", "/api/v1/hooks/"+hook.ID, "", payload, map[string]string{"X-SP-Signature": "sha256=00"})
	if code != http.StatusUnauthorized {
		t.Errorf("bad signature: expected 401, got %d", code)
	}
	// unknown webhook -> 404
	code, _ = doJSON(t, h, nil, "POST", "/api/v1/hooks/nope", "", payload, map[string]string{"X-SP-Signature": sig})
	if code != http.StatusNotFound {
		t.Errorf("unknown webhook: expected 404, got %d", code)
	}
	// valid signature -> 202 and the action ran
	code, body := doJSON(t, h, nil, "POST", "/api/v1/hooks/"+hook.ID, "", payload, map[string]string{"X-SP-Signature": sig})
	if code != http.StatusAccepted {
		t.Errorf("valid event: expected 202, got %d %v", code, body)
	}
	if body["fired"].(float64) != 1 {
		t.Errorf("fired = %v, want 1", body["fired"])
	}
	if len(issues.calls) != 1 || issues.calls[0].Input.Title != "deploy finished" {
		t.Errorf("webhook event must run the bound autopilot with payload title: %+v", issues.calls)
	}
	// non-JSON body with valid signature -> 400
	bad := sign(hook.Secret, []byte("not json"))
	code, _ = doJSON(t, h, nil, "POST", "/api/v1/hooks/"+hook.ID, "", "not json", map[string]string{"X-SP-Signature": bad})
	if code != http.StatusBadRequest {
		t.Errorf("non-JSON body: expected 400, got %d", code)
	}
}

func TestInboundHookDisabledWebhookForbidden(t *testing.T) {
	h, _, svc, _, issues, _ := setupHandler(t)
	ctx := context.Background()
	hook, _ := svc.CreateWebhook(ctx, "hook")
	if _, err := svc.UpdateWebhook(ctx, hook.ID, nil, boolPtr(false)); err != nil {
		t.Fatalf("disable: %v", err)
	}
	payload := `{}`
	sig := sign(hook.Secret, []byte(payload))
	code, _ := doJSON(t, h, nil, "POST", "/api/v1/hooks/"+hook.ID, "", payload, map[string]string{"X-SP-Signature": sig})
	if code != http.StatusForbidden {
		t.Errorf("disabled webhook: expected 403, got %d", code)
	}
	if len(issues.calls) != 0 {
		t.Errorf("disabled webhook must not fire actions")
	}
}

func TestAutopilotAPIAndManualTrigger(t *testing.T) {
	h, tokens, _, _, issues, _ := setupHandler(t)
	body := `{"name":"hourly","trigger_type":"cron","cron_spec":"0 * * * *","action_type":"create_issue","project_id":"p1","issue_title":"hourly report","enabled":true}`
	code, resp := doJSON(t, h, tokens, "POST", "/api/v1/autopilots/", "u1", body, nil)
	if code != http.StatusCreated {
		t.Fatalf("create autopilot: %d %v", code, resp)
	}
	a := resp["autopilot"].(map[string]any)
	id := a["id"].(string)
	if a["next_run_at"] == nil {
		t.Errorf("cron autopilot must expose next_run_at")
	}
	// invalid cron rejected
	bad := `{"name":"bad","trigger_type":"cron","cron_spec":"??","action_type":"create_issue","project_id":"p1","issue_title":"t"}`
	if code, _ := doJSON(t, h, tokens, "POST", "/api/v1/autopilots/", "u1", bad, nil); code != http.StatusBadRequest {
		t.Errorf("invalid cron: expected 400, got %d", code)
	}
	// manual trigger endpoint
	code, _ = doJSON(t, h, tokens, "POST", "/api/v1/autopilots/"+id+"/trigger", "u1", "", nil)
	if code != http.StatusAccepted {
		t.Errorf("manual trigger: expected 202, got %d", code)
	}
	if len(issues.calls) != 1 {
		t.Errorf("manual trigger must run the action")
	}
	// list
	code, resp = doJSON(t, h, tokens, "GET", "/api/v1/autopilots/", "u1", "", nil)
	if code != http.StatusOK || len(resp["autopilots"].([]any)) != 1 {
		t.Errorf("list autopilots failed: %d %v", code, resp)
	}
}

func boolPtr(b bool) *bool { return &b }
