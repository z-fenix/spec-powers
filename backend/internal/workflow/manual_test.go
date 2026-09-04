package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/skill"
)

// StartChange with manual=true creates a change without the AI splitter so
// the agent-driven skill flow can start from a bare proposal-phase change.
func TestStartChangeManual(t *testing.T) {
	f := newFixture()

	t.Run("member starts a bare change", func(t *testing.T) {
		c, _, err := f.svc.StartChange(context.Background(), "alice", "i1", true)
		if err != nil {
			t.Fatalf("start change: %v", err)
		}
		if c.ID == "" || c.IssueID != "i1" || c.ProjectID != "p1" {
			t.Errorf("change = %+v", c)
		}
		if c.Phase != KindProposal {
			t.Errorf("Phase = %q, want %q", c.Phase, KindProposal)
		}
		if c.Status != "active" {
			t.Errorf("Status = %q, want active", c.Status)
		}
		if c.CreatedBy != "alice" {
			t.Errorf("CreatedBy = %q, want alice", c.CreatedBy)
		}
	})

	t.Run("issue already in a change conflicts", func(t *testing.T) {
		f.changes.byIssue["i1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1"}
		_, _, err := f.svc.StartChange(context.Background(), "alice", "i1", true)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 409 {
			t.Errorf("error = %v, want 409", err)
		}
	})

	t.Run("unknown issue is not found", func(t *testing.T) {
		_, _, err := f.svc.StartChange(context.Background(), "alice", "nope", true)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f.issues.byID["i2"] = &domain.Issue{ID: "i2", ProjectID: "p1"}
		_, _, err := f.svc.StartChange(context.Background(), "eve", "i2", true)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})

	t.Run("works without a splitter attached", func(t *testing.T) {
		if f.svc.splitter != nil {
			t.Fatal("fixture must not attach a splitter")
		}
		f.issues.byID["i3"] = &domain.Issue{ID: "i3", ProjectID: "p1"}
		if _, _, err := f.svc.StartChange(context.Background(), "alice", "i3", true); err != nil {
			t.Errorf("manual start without splitter: %v", err)
		}
	})
}

func (f *handlerFixture) doBody(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func TestStartChangeEndpointManual(t *testing.T) {
	h := setupHandler(t)
	tok := h.token(t, "alice")

	t.Run("manual start creates a bare change", func(t *testing.T) {
		w := h.doBody(t, http.MethodPost, "/changes", tok, `{"issue_id":"i1","manual":true}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Change changeDTO `json:"change"`
			Tasks  []taskDTO `json:"tasks"`
		}
		decode(t, w, &body)
		if body.Change.Phase != KindProposal || body.Change.Status != "active" {
			t.Errorf("change = %+v", body.Change)
		}
		if body.Tasks == nil || len(body.Tasks) != 0 {
			t.Errorf("tasks = %+v, want empty", body.Tasks)
		}
	})

	t.Run("without manual the splitter path applies", func(t *testing.T) {
		h.f.issues.byID["i2"] = &domain.Issue{ID: "i2", ProjectID: "p1"}
		w := h.doBody(t, http.MethodPost, "/changes", tok, `{"issue_id":"i2"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (splitter not configured), body=%s", w.Code, w.Body.String())
		}
	})
}

const manualTasksMD = "```json\n{\"tasks\":[" +
	`{"title":"任务一","description":"先做一","stage":1},` +
	`{"title":"任务二","description":"再做二","stage":2}` +
	"]}\n```"

func TestWriteArtifact(t *testing.T) {
	f := newFixture()
	creator := &splitCreator{}
	f.svc = f.svc.WithCreator(creator)
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: "proposal", Status: "active"}
	f.issues.byID["i1"] = &domain.Issue{ID: "i1", ProjectID: "p1"}

	t.Run("member writes a proposal artifact", func(t *testing.T) {
		a, err := f.svc.WriteArtifact(context.Background(), "alice", "c1", KindProposal, "# 提案")
		if err != nil {
			t.Fatalf("write artifact: %v", err)
		}
		if a.Kind != KindProposal || a.Version != 1 || a.Content != "# 提案" || a.CreatedBy != "alice" {
			t.Errorf("artifact = %+v", a)
		}
	})

	t.Run("rewriting bumps the version", func(t *testing.T) {
		a, err := f.svc.WriteArtifact(context.Background(), "alice", "c1", KindProposal, "# 提案 v2")
		if err != nil {
			t.Fatalf("rewrite artifact: %v", err)
		}
		if a.Version != 2 {
			t.Errorf("Version = %d, want 2", a.Version)
		}
	})

	t.Run("tasks write creates sub-issues and mappings", func(t *testing.T) {
		a, err := f.svc.WriteArtifact(context.Background(), "alice", "c1", KindTasks, manualTasksMD)
		if err != nil {
			t.Fatalf("write tasks: %v", err)
		}
		if len(creator.created) != 2 {
			t.Fatalf("created %d sub-issues, want 2", len(creator.created))
		}
		if creator.created[0].ParentID != "i1" || creator.created[0].Stage != 1 {
			t.Errorf("first sub-issue = %+v", creator.created[0])
		}
		ms := f.mappings.byChange["c1"]
		if len(ms) != 2 || ms[0].ArtifactID != a.ID || ms[0].IssueID == "" {
			t.Errorf("mappings = %+v", ms)
		}
	})

	t.Run("tasks write with invalid json is rejected", func(t *testing.T) {
		_, err := f.svc.WriteArtifact(context.Background(), "alice", "c1", KindTasks, "没有 json 块")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Errorf("error = %v, want 400", err)
		}
	})

	t.Run("unknown kind is invalid", func(t *testing.T) {
		_, err := f.svc.WriteArtifact(context.Background(), "alice", "c1", "verify", "x")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Errorf("error = %v, want 400", err)
		}
	})

	t.Run("empty content is invalid", func(t *testing.T) {
		_, err := f.svc.WriteArtifact(context.Background(), "alice", "c1", KindSpecs, "  ")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Errorf("error = %v, want 400", err)
		}
	})

	t.Run("archived change conflicts", func(t *testing.T) {
		f.changes.byID["c2"] = &domain.Change{ID: "c2", ProjectID: "p1", IssueID: "i1", Phase: "tasks", Status: "archived"}
		_, err := f.svc.WriteArtifact(context.Background(), "alice", "c2", KindProposal, "x")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 409 {
			t.Errorf("error = %v, want 409", err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.WriteArtifact(context.Background(), "eve", "c1", KindProposal, "x")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})
}

func TestWriteArtifactEndpoint(t *testing.T) {
	h := setupHandler(t)
	f := h.f
	f.svc = f.svc.WithCreator(&splitCreator{})
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: "proposal", Status: "active"}
	f.issues.byID["i1"] = &domain.Issue{ID: "i1", ProjectID: "p1"}
	tok := h.token(t, "bob")

	t.Run("writes an artifact", func(t *testing.T) {
		w := h.doBody(t, http.MethodPost, "/changes/c1/artifacts/proposal", tok, `{"content":"# 提案"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Artifact artifactDTO `json:"artifact"`
		}
		decode(t, w, &body)
		if body.Artifact.Kind != KindProposal || body.Artifact.Version != 1 {
			t.Errorf("artifact = %+v", body.Artifact)
		}
	})

	t.Run("unknown kind is invalid", func(t *testing.T) {
		w := h.doBody(t, http.MethodPost, "/changes/c1/artifacts/nope", tok, `{"content":"x"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing content is invalid", func(t *testing.T) {
		w := h.doBody(t, http.MethodPost, "/changes/c1/artifacts/proposal", tok, `{}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("requires auth", func(t *testing.T) {
		w := h.doBody(t, http.MethodPost, "/changes/c1/artifacts/proposal", "", `{"content":"x"}`)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})
}

func TestNextSkill(t *testing.T) {
	reg, err := skill.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	f := newFixture()
	f.svc = f.svc.WithSkills(reg)
	f.projects.existing["p1"] = true

	t.Run("proposal phase yields brainstorm", func(t *testing.T) {
		f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: KindProposal, Status: "active"}
		s, err := f.svc.NextSkill(context.Background(), "alice", "c1")
		if err != nil {
			t.Fatalf("next skill: %v", err)
		}
		if s.Key != skill.KeyBrainstorm || s.Instructions == "" {
			t.Errorf("skill = %+v", s)
		}
	})

	t.Run("design phase yields write-plan", func(t *testing.T) {
		f.changes.byID["c2"] = &domain.Change{ID: "c2", ProjectID: "p1", IssueID: "i1", Phase: KindDesign, Status: "active"}
		s, err := f.svc.NextSkill(context.Background(), "alice", "c2")
		if err != nil {
			t.Fatalf("next skill: %v", err)
		}
		if s.Key != skill.KeyWritePlan {
			t.Errorf("key = %q, want %q", s.Key, skill.KeyWritePlan)
		}
	})

	t.Run("tasks phase yields subagent-driven-development", func(t *testing.T) {
		f.changes.byID["c3"] = &domain.Change{ID: "c3", ProjectID: "p1", IssueID: "i1", Phase: KindTasks, Status: "active"}
		s, err := f.svc.NextSkill(context.Background(), "alice", "c3")
		if err != nil {
			t.Fatalf("next skill: %v", err)
		}
		if s.Key != skill.KeySubagentDrivenDevelopment {
			t.Errorf("key = %q, want %q", s.Key, skill.KeySubagentDrivenDevelopment)
		}
	})

	t.Run("archived change has no next skill", func(t *testing.T) {
		f.changes.byID["c4"] = &domain.Change{ID: "c4", ProjectID: "p1", IssueID: "i1", Phase: KindTasks, Status: "archived"}
		_, err := f.svc.NextSkill(context.Background(), "alice", "c4")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})

	t.Run("unknown change is not found", func(t *testing.T) {
		_, err := f.svc.NextSkill(context.Background(), "alice", "nope")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Errorf("error = %v, want 404", err)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.NextSkill(context.Background(), "eve", "c1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Errorf("error = %v, want 403", err)
		}
	})
}

func TestNextSkillEndpoint(t *testing.T) {
	h := setupHandler(t)
	reg, err := skill.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	h.f.svc = h.f.svc.WithSkills(reg)
	h.f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: KindProposal, Status: "active"}
	tok := h.token(t, "bob")

	t.Run("returns the next skill for the change", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/c1/skills/next", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Skill skill.Skill `json:"skill"`
		}
		decode(t, w, &body)
		if body.Skill.Key != skill.KeyBrainstorm || body.Skill.Instructions == "" {
			t.Errorf("skill = %+v", body.Skill)
		}
	})

	t.Run("unknown change is not found", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/nope/skills/next", tok)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}
