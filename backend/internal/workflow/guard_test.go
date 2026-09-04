package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

// ---- guard fixture helpers ----

// gateTime is the base instant for the seeded classic change timeline.
var gateTime = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

func art(id, changeID, kind string, version int, at time.Time) domain.Artifact {
	return domain.Artifact{ID: id, ChangeID: changeID, Kind: kind, Version: version, Content: "# " + kind, CreatedAt: at}
}

func handoff(id, changeID, from, to string, at time.Time) domain.ChangeHandoff {
	return domain.ChangeHandoff{ID: id, ChangeID: changeID, FromPhase: from, ToPhase: to, CreatedBy: "bob", CreatedAt: at}
}

// seedClassicChange seeds a change that completed the AI split: phase tasks,
// four artifacts, handoffs for every advance. Verify reports are added by
// the tests that need them.
func seedClassicChange(f *fixture, status, phase string) {
	f.issues.byID["i1"] = &domain.Issue{ID: "i1", ProjectID: "p1"}
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: phase, Status: status}
	f.changes.byIssue["i1"] = f.changes.byID["c1"]
	f.artifacts.latest["c1"] = []domain.Artifact{
		art("a-proposal", "c1", KindProposal, 1, gateTime),
		art("a-specs", "c1", KindSpecs, 1, gateTime.Add(2*time.Hour)),
		art("a-design", "c1", KindDesign, 1, gateTime.Add(4*time.Hour)),
		art("a-tasks", "c1", KindTasks, 1, gateTime.Add(6*time.Hour)),
	}
	f.artifacts.byKind[KindProposal] = []domain.Artifact{art("a-proposal", "c1", KindProposal, 1, gateTime)}
	f.artifacts.byKind[KindSpecs] = []domain.Artifact{art("a-specs", "c1", KindSpecs, 1, gateTime.Add(2*time.Hour))}
	f.artifacts.byKind[KindDesign] = []domain.Artifact{art("a-design", "c1", KindDesign, 1, gateTime.Add(4*time.Hour))}
	f.artifacts.byKind[KindTasks] = []domain.Artifact{art("a-tasks", "c1", KindTasks, 1, gateTime.Add(6*time.Hour))}
	f.changes.handoffs["c1"] = []domain.ChangeHandoff{
		handoff("h3", "c1", KindDesign, KindTasks, gateTime.Add(5*time.Hour)),
		handoff("h2", "c1", KindSpecs, KindDesign, gateTime.Add(3*time.Hour)),
		handoff("h1", "c1", KindProposal, KindSpecs, gateTime.Add(1*time.Hour)),
	}
}

func seedVerifyReport(f *fixture, content string, at time.Time) {
	a := domain.Artifact{ID: "a-verify", ChangeID: "c1", Kind: KindVerify, Version: 1, Content: content, CreatedAt: at}
	f.artifacts.byKind[KindVerify] = []domain.Artifact{a}
	f.artifacts.latest["c1"] = append(f.artifacts.latest["c1"], a)
}

func appErrStatus(t *testing.T, err error) int {
	t.Helper()
	appErr, ok := err.(*httpapi.AppError)
	if !ok {
		t.Fatalf("error = %v (%T), want AppError", err, err)
	}
	return appErr.Status
}

// ---- verify report parsing ----

func TestParseVerifyReport(t *testing.T) {
	t.Run("accepts pass", func(t *testing.T) {
		rep, err := ParseVerifyReport("result: pass\nsummary: all green\n")
		if err != nil || rep.Result != "pass" {
			t.Errorf("rep = %+v, err = %v", rep, err)
		}
	})

	t.Run("accepts fail", func(t *testing.T) {
		rep, err := ParseVerifyReport("result: fail\n")
		if err != nil || rep.Result != "fail" {
			t.Errorf("rep = %+v, err = %v", rep, err)
		}
	})

	t.Run("rejects malformed YAML", func(t *testing.T) {
		if _, err := ParseVerifyReport("result: [pass\n  broken"); err == nil {
			t.Error("expected error for malformed YAML")
		}
	})

	t.Run("rejects non-mapping document", func(t *testing.T) {
		if _, err := ParseVerifyReport("- just\n- a\n- list\n"); err == nil {
			t.Error("expected error for non-mapping YAML")
		}
	})

	t.Run("rejects missing result", func(t *testing.T) {
		if _, err := ParseVerifyReport("summary: no result field\n"); err == nil {
			t.Error("expected error for missing result")
		}
	})

	t.Run("rejects unknown result value", func(t *testing.T) {
		if _, err := ParseVerifyReport("result: ok\n"); err == nil {
			t.Error("expected error for unknown result value")
		}
	})
}

// ---- guard status ----

func TestGuardStatusArchivableChange(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)
	seedVerifyReport(f, "result: pass\n", gateTime.Add(7*time.Hour))

	rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
	if err != nil {
		t.Fatalf("guard status: %v", err)
	}
	if !rep.PhaseLegal || !rep.HandoffFresh || !rep.VerifyPassed {
		t.Errorf("gates = legal:%v fresh:%v verify:%v reasons:%v",
			rep.PhaseLegal, rep.HandoffFresh, rep.VerifyPassed, rep.Reasons)
	}
	if !rep.CanArchive {
		t.Errorf("CanArchive = false, want true for a fully gated change (reasons: %v)", rep.Reasons)
	}
	if rep.CanAdvance {
		t.Errorf("CanAdvance = true, want false at tasks phase")
	}
	if rep.NextPhase != "" {
		t.Errorf("NextPhase = %q, want empty at tasks phase", rep.NextPhase)
	}
	if len(rep.Reasons) != 0 {
		t.Errorf("reasons = %v, want none", rep.Reasons)
	}
}

func TestGuardStatusPhaseLegality(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindDesign)
	// drop the specs artifact: phase design without specs is an illegal skip
	f.artifacts.latest["c1"] = []domain.Artifact{
		art("a-proposal", "c1", KindProposal, 1, gateTime),
		art("a-design", "c1", KindDesign, 1, gateTime.Add(4*time.Hour)),
	}

	rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
	if err != nil {
		t.Fatalf("guard status: %v", err)
	}
	if rep.PhaseLegal {
		t.Error("PhaseLegal = true, want false when specs artifact is missing")
	}
	if rep.CanAdvance || rep.CanArchive {
		t.Errorf("CanAdvance/CanArchive = %v/%v, want false", rep.CanAdvance, rep.CanArchive)
	}
	if len(rep.Reasons) == 0 {
		t.Error("reasons should name the missing artifact")
	}
}

func TestGuardStatusHandoffFreshness(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)

	t.Run("missing handoff for current phase", func(t *testing.T) {
		f.changes.handoffs["c1"] = nil
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.HandoffFresh {
			t.Error("HandoffFresh = true, want false with no handoffs")
		}
	})

	t.Run("handoff does not match current phase", func(t *testing.T) {
		f.changes.handoffs["c1"] = []domain.ChangeHandoff{
			handoff("h2", "c1", KindSpecs, KindDesign, gateTime.Add(3*time.Hour)),
		}
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.HandoffFresh {
			t.Error("HandoffFresh = true, want false when latest handoff targets another phase")
		}
	})

	t.Run("artifact regenerated after handoff", func(t *testing.T) {
		stale := art("a-proposal-v2", "c1", KindProposal, 2, gateTime.Add(8*time.Hour))
		f.artifacts.byKind[KindProposal] = []domain.Artifact{stale,
			art("a-proposal", "c1", KindProposal, 1, gateTime)}
		f.artifacts.latest["c1"] = []domain.Artifact{
			stale,
			art("a-specs", "c1", KindSpecs, 1, gateTime.Add(2*time.Hour)),
			art("a-design", "c1", KindDesign, 1, gateTime.Add(4*time.Hour)),
			art("a-tasks", "c1", KindTasks, 1, gateTime.Add(6*time.Hour)),
		}
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.HandoffFresh {
			t.Error("HandoffFresh = true, want false when an earlier-phase artifact was regenerated after the handoff")
		}
	})
}

func TestGuardStatusVerifyGate(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)

	t.Run("no verify report", func(t *testing.T) {
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.VerifyPassed {
			t.Error("VerifyPassed = true, want false without a verify report")
		}
	})

	t.Run("fail result does not pass", func(t *testing.T) {
		seedVerifyReport(f, "result: fail\n", gateTime.Add(7*time.Hour))
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.VerifyPassed {
			t.Error("VerifyPassed = true, want false for result: fail")
		}
	})

	t.Run("malformed report does not pass", func(t *testing.T) {
		seedVerifyReport(f, "not: [valid\n", gateTime.Add(7*time.Hour))
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.VerifyPassed {
			t.Error("VerifyPassed = true, want false for malformed report")
		}
	})
}

func TestGuardStatusArchivedChange(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "archived", KindTasks)
	seedVerifyReport(f, "result: pass\n", gateTime.Add(7*time.Hour))

	rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
	if err != nil {
		t.Fatalf("guard status: %v", err)
	}
	if rep.CanAdvance || rep.CanArchive {
		t.Errorf("CanAdvance/CanArchive = %v/%v, want false for archived change", rep.CanAdvance, rep.CanArchive)
	}
}

// seedSpecsChange seeds a legal change in the specs phase: proposal and
// specs artifacts exist and the entry handoff is recorded.
func seedSpecsChange(f *fixture) {
	f.issues.byID["i1"] = &domain.Issue{ID: "i1", ProjectID: "p1"}
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: KindSpecs, Status: "active"}
	f.changes.byIssue["i1"] = f.changes.byID["c1"]
	f.artifacts.latest["c1"] = []domain.Artifact{
		art("a-proposal", "c1", KindProposal, 1, gateTime),
		art("a-specs", "c1", KindSpecs, 1, gateTime.Add(2*time.Hour)),
	}
	f.artifacts.byKind[KindProposal] = []domain.Artifact{art("a-proposal", "c1", KindProposal, 1, gateTime)}
	f.artifacts.byKind[KindSpecs] = []domain.Artifact{art("a-specs", "c1", KindSpecs, 1, gateTime.Add(2*time.Hour))}
	f.changes.handoffs["c1"] = []domain.ChangeHandoff{
		handoff("h1", "c1", KindProposal, KindSpecs, gateTime.Add(1*time.Hour)),
	}
}

// ---- advance phase ----

func TestAdvancePhase(t *testing.T) {
	f := newFixture()
	seedSpecsChange(f)

	change, h, err := f.svc.AdvancePhase(context.Background(), "bob", "c1")
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if change.Phase != KindDesign {
		t.Errorf("phase = %q, want design", change.Phase)
	}
	if h.FromPhase != KindSpecs || h.ToPhase != KindDesign || h.CreatedBy != "bob" {
		t.Errorf("handoff = %+v", h)
	}
	if got := f.changes.handoffs["c1"]; len(got) != 2 || got[0].FromPhase != KindSpecs || got[0].ToPhase != KindDesign {
		t.Errorf("stored handoffs = %+v", got)
	}
}

func TestAdvancePhaseBlockedAtFinalPhase(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)

	_, _, err := f.svc.AdvancePhase(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
}

func TestAdvancePhaseBlockedByStaleHandoff(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindDesign)
	stale := art("a-proposal-v2", "c1", KindProposal, 2, gateTime.Add(8*time.Hour))
	f.artifacts.latest["c1"] = append(f.artifacts.latest["c1"], stale)
	f.artifacts.byKind[KindProposal] = []domain.Artifact{stale,
		art("a-proposal", "c1", KindProposal, 1, gateTime)}

	_, _, err := f.svc.AdvancePhase(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
}

func TestAdvancePhaseBlockedWhenNotActive(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "failed", KindSpecs)

	_, _, err := f.svc.AdvancePhase(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
}

// ---- verify report submission ----

func TestSubmitVerifyReport(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)

	t.Run("pass report is stored and releases the gate", func(t *testing.T) {
		a, passed, err := f.svc.SubmitVerifyReport(context.Background(), "bob", "c1",
			"result: pass\nsummary: all checks green\n")
		if err != nil || !passed {
			t.Fatalf("submit = %+v, passed %v, err %v", a, passed, err)
		}
		if a.Kind != KindVerify || a.Version != 1 || a.CreatedBy != "bob" {
			t.Errorf("artifact = %+v", a)
		}
	})

	t.Run("newer fail report flips the gate off", func(t *testing.T) {
		_, passed, err := f.svc.SubmitVerifyReport(context.Background(), "bob", "c1", "result: fail\n")
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if passed {
			t.Error("passed = true, want false for fail report")
		}
		rep, err := f.svc.GuardStatus(context.Background(), "bob", "c1")
		if err != nil {
			t.Fatalf("guard status: %v", err)
		}
		if rep.VerifyPassed {
			t.Error("VerifyPassed = true, want false after latest fail report")
		}
	})

	t.Run("malformed report is rejected", func(t *testing.T) {
		_, _, err := f.svc.SubmitVerifyReport(context.Background(), "bob", "c1", "result: [broken")
		if status := appErrStatus(t, err); status != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", status)
		}
	})

	t.Run("archived change rejects reports", func(t *testing.T) {
		f.changes.byID["c1"].Status = "archived"
		_, _, err := f.svc.SubmitVerifyReport(context.Background(), "bob", "c1", "result: pass\n")
		if status := appErrStatus(t, err); status != http.StatusConflict {
			t.Errorf("status = %d, want 409", status)
		}
	})
}

// ---- archive ----

type fakeWakeups struct {
	recorded []string // "issueID|childIssueID"
}

func (f *fakeWakeups) CreateIssueWakeup(_ context.Context, issueID, childIssueID string) error {
	f.recorded = append(f.recorded, issueID+"|"+childIssueID)
	return nil
}

func archivedFixture() (*fixture, *fakeWakeups) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)
	seedVerifyReport(f, "result: pass\n", gateTime.Add(7*time.Hour))
	w := &fakeWakeups{}
	f.svc = f.svc.WithWaker(w)
	return f, w
}

func TestArchiveChange(t *testing.T) {
	f, _ := archivedFixture()

	change, err := f.svc.Archive(context.Background(), "bob", "c1")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if change.Status != "archived" {
		t.Errorf("status = %q, want archived", change.Status)
	}
	if stored := f.changes.byID["c1"].Status; stored != "archived" {
		t.Errorf("stored status = %q, want archived", stored)
	}
}

func TestArchiveWakesParentIssueOwner(t *testing.T) {
	f, w := archivedFixture()
	f.issues.byID["i1"].ParentID = "root-issue"

	if _, err := f.svc.Archive(context.Background(), "bob", "c1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(w.recorded) != 1 || w.recorded[0] != "root-issue|i1" {
		t.Errorf("wakeups = %v, want [root-issue|i1]", w.recorded)
	}
}

type fakeWakeupHook struct{ parents []*domain.Issue }

func (h *fakeWakeupHook) OnParentWakeup(_ context.Context, parent *domain.Issue) error {
	h.parents = append(h.parents, parent)
	return nil
}

func TestArchiveNotifiesParentWakeupHook(t *testing.T) {
	f, w := archivedFixture()
	f.issues.byID["i1"].ParentID = "root-issue"
	f.issues.byID["root-issue"] = &domain.Issue{ID: "root-issue", Title: "root", AssigneeID: "human-1"}
	hook := &fakeWakeupHook{}
	f.svc = f.svc.WithWakeupHook(hook)

	if _, err := f.svc.Archive(context.Background(), "bob", "c1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(w.recorded) != 1 {
		t.Fatalf("wakeups = %v, want recorded", w.recorded)
	}
	if len(hook.parents) != 1 || hook.parents[0].ID != "root-issue" || hook.parents[0].AssigneeID != "human-1" {
		t.Errorf("hook parents = %+v, want [root-issue]", hook.parents)
	}
}

func TestArchiveWithoutParentRecordsNoWakeup(t *testing.T) {
	f, w := archivedFixture()

	if _, err := f.svc.Archive(context.Background(), "bob", "c1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(w.recorded) != 0 {
		t.Errorf("wakeups = %v, want none", w.recorded)
	}
}

func TestArchiveBlockedWhenVerifyNotPassed(t *testing.T) {
	f, _ := archivedFixture()
	seedVerifyReport(f, "result: fail\n", gateTime.Add(7*time.Hour))

	_, err := f.svc.Archive(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
	if stored := f.changes.byID["c1"].Status; stored != "active" {
		t.Errorf("status = %q, want unchanged", stored)
	}
}

func TestArchiveBlockedBeforeTasksPhase(t *testing.T) {
	f, _ := archivedFixture()
	seedClassicChange(f, "active", KindDesign)

	_, err := f.svc.Archive(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
}

func TestArchiveBlockedTwice(t *testing.T) {
	f, _ := archivedFixture()

	if _, err := f.svc.Archive(context.Background(), "bob", "c1"); err != nil {
		t.Fatalf("first archive: %v", err)
	}
	_, err := f.svc.Archive(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusConflict {
		t.Errorf("second archive status = %d, want 409", status)
	}
}

func TestArchiveWithoutWakerIsInternalError(t *testing.T) {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)
	seedVerifyReport(f, "result: pass\n", gateTime.Add(7*time.Hour))

	_, err := f.svc.Archive(context.Background(), "bob", "c1")
	if status := appErrStatus(t, err); status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when wakeup recorder is not wired", status)
	}
}

func TestArchiveForbiddenForNonMember(t *testing.T) {
	f, _ := archivedFixture()

	_, err := f.svc.Archive(context.Background(), "eve", "c1")
	if status := appErrStatus(t, err); status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", status)
	}
}

// ---- HTTP endpoints ----

func (fh *handlerFixture) doWithBody(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	fh.handler.ServeHTTP(w, req)
	return w
}

// routerFor builds the handler routes for a fixture's service, mirroring the
// production mount under /changes.
func routerFor(f *fixture, tokens *auth.TokenService) http.Handler {
	h := NewHandler(f.svc, tokens)
	r := chi.NewRouter()
	r.Route("/changes", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	return r
}

func guardHandlerSeed() *fixture {
	f := newFixture()
	seedClassicChange(f, "active", KindTasks)
	seedVerifyReport(f, "result: pass\n", gateTime.Add(7*time.Hour))
	return f
}

func TestGuardEndpoints(t *testing.T) {
	t.Run("GET guard report", func(t *testing.T) {
		f := guardHandlerSeed()
		h := setupHandler(t)
		h.f = f
		h.handler = routerFor(f, h.tokens)
		tok := h.token(t, "bob")

		w := h.do(t, http.MethodGet, "/changes/c1/guard", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Guard GuardReport `json:"guard"`
		}
		decode(t, w, &body)
		if body.Guard.ChangeID != "c1" || !body.Guard.VerifyPassed || body.Guard.Phase != KindTasks {
			t.Errorf("guard = %+v", body.Guard)
		}
	})

	t.Run("POST verify stores report", func(t *testing.T) {
		f := guardHandlerSeed()
		h := setupHandler(t)
		h.f = f
		h.handler = routerFor(f, h.tokens)
		tok := h.token(t, "bob")

		w := h.doWithBody(t, http.MethodPost, "/changes/c1/verify", tok, `{"content":"result: pass\n"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Artifact artifactDTO `json:"artifact"`
			Result   string      `json:"result"`
			Passed   bool        `json:"passed"`
		}
		decode(t, w, &body)
		if body.Artifact.Kind != KindVerify || body.Result != "pass" || !body.Passed {
			t.Errorf("body = %+v", body)
		}
	})

	t.Run("POST verify rejects malformed YAML", func(t *testing.T) {
		f := guardHandlerSeed()
		h := setupHandler(t)
		h.f = f
		h.handler = routerFor(f, h.tokens)
		tok := h.token(t, "bob")

		w := h.doWithBody(t, http.MethodPost, "/changes/c1/verify", tok, `{"content":"result: [broken"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("POST guard advances phase", func(t *testing.T) {
		f := newFixture()
		seedSpecsChange(f)
		h := setupHandler(t)
		h.f = f
		h.handler = routerFor(f, h.tokens)
		tok := h.token(t, "bob")

		w := h.do(t, http.MethodPost, "/changes/c1/guard", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Change  changeDTO  `json:"change"`
			Handoff handoffDTO `json:"handoff"`
		}
		decode(t, w, &body)
		if body.Change.Phase != KindDesign || body.Handoff.ToPhase != KindDesign {
			t.Errorf("body = %+v", body)
		}
	})

	t.Run("POST archive archives change", func(t *testing.T) {
		f := guardHandlerSeed()
		f.svc = f.svc.WithWaker(&fakeWakeups{})
		h := setupHandler(t)
		h.f = f
		h.handler = routerFor(f, h.tokens)
		tok := h.token(t, "bob")

		w := h.do(t, http.MethodPost, "/changes/c1/archive", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Change changeDTO `json:"change"`
		}
		decode(t, w, &body)
		if body.Change.Status != "archived" {
			t.Errorf("change = %+v", body.Change)
		}
	})

	t.Run("new endpoints are forbidden for non-members", func(t *testing.T) {
		f := guardHandlerSeed()
		h := setupHandler(t)
		h.f = f
		h.handler = routerFor(f, h.tokens)
		tok := h.token(t, "eve")

		for _, path := range []string{"/changes/c1/guard", "/changes/c1/archive"} {
			if w := h.do(t, http.MethodPost, path, tok); w.Code != http.StatusForbidden {
				t.Errorf("POST %s status = %d, want 403", path, w.Code)
			}
		}
		if w := h.doWithBody(t, http.MethodPost, "/changes/c1/verify", tok, `{"content":"result: pass\n"}`); w.Code != http.StatusForbidden {
			t.Errorf("POST /changes/c1/verify status = %d, want 403", w.Code)
		}
	})
}
