package workflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
)

type handlerFixture struct {
	handler http.Handler
	tokens  *auth.TokenService
	f       *fixture
}

func setupHandler(t *testing.T) *handlerFixture {
	t.Helper()
	f := newFixture()
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(f.svc, tokens)

	// mirror the production mount: workflow lives under /api/v1/changes
	r := chi.NewRouter()
	r.Route("/changes", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	return &handlerFixture{handler: r, tokens: tokens, f: f}
}

func (f *handlerFixture) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := f.tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (f *handlerFixture) do(t *testing.T, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
}

func seedHandlerData(f *fixture) {
	f.changes.byID["c1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: "tasks", Status: "active"}
	f.changes.byIssue["i1"] = f.changes.byID["c1"]
	f.artifacts.latest["c1"] = []domain.Artifact{
		{ID: "a1", ChangeID: "c1", Kind: "proposal", Version: 2, Content: "# v2"},
	}
	f.artifacts.byKind["proposal"] = []domain.Artifact{
		{ID: "a1", ChangeID: "c1", Kind: "proposal", Version: 2, Content: "# v2"},
		{ID: "a0", ChangeID: "c1", Kind: "proposal", Version: 1, Content: "# v1"},
	}
	f.mappings.byChange["c1"] = []domain.TaskMapping{
		{ID: "m1", ChangeID: "c1", ArtifactID: "a9", IssueID: "i2", Title: "child", Stage: 1, Position: 0},
	}
}

func TestChangeHandlerAuth(t *testing.T) {
	h := setupHandler(t)
	w := h.do(t, http.MethodGet, "/changes/c1", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestChangeByIssueEndpoint(t *testing.T) {
	h := setupHandler(t)
	seedHandlerData(h.f)
	tok := h.token(t, "bob")

	t.Run("returns change for issue", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes?issue_id=i1", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Change changeDTO `json:"change"`
		}
		decode(t, w, &body)
		if body.Change.ID != "c1" || body.Change.IssueID != "i1" ||
			body.Change.Phase != "tasks" || body.Change.Status != "active" {
			t.Errorf("change = %+v", body.Change)
		}
	})

	t.Run("missing issue_id is invalid", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes", tok)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("issue without change is 404", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes?issue_id=nope", tok)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestGetChangeEndpoint(t *testing.T) {
	h := setupHandler(t)
	seedHandlerData(h.f)
	tok := h.token(t, "bob")

	w := h.do(t, http.MethodGet, "/changes/c1", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Change changeDTO `json:"change"`
	}
	decode(t, w, &body)
	if body.Change.ID != "c1" || body.Change.ProjectID != "p1" {
		t.Errorf("change = %+v", body.Change)
	}

	if w := h.do(t, http.MethodGet, "/changes/nope", tok); w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestListArtifactsEndpoint(t *testing.T) {
	h := setupHandler(t)
	seedHandlerData(h.f)
	tok := h.token(t, "bob")

	w := h.do(t, http.MethodGet, "/changes/c1/artifacts", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Artifacts []artifactDTO `json:"artifacts"`
	}
	decode(t, w, &body)
	if len(body.Artifacts) != 1 || body.Artifacts[0].Kind != "proposal" ||
		body.Artifacts[0].Version != 2 || body.Artifacts[0].Content != "# v2" {
		t.Errorf("artifacts = %+v", body.Artifacts)
	}
}

func TestGetArtifactEndpoint(t *testing.T) {
	h := setupHandler(t)
	seedHandlerData(h.f)
	tok := h.token(t, "bob")

	t.Run("latest by default", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/c1/artifacts/proposal", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Artifact artifactDTO `json:"artifact"`
		}
		decode(t, w, &body)
		if body.Artifact.Version != 2 || body.Artifact.Content != "# v2" {
			t.Errorf("artifact = %+v", body.Artifact)
		}
	})

	t.Run("specific version", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/c1/artifacts/proposal?version=1", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Artifact artifactDTO `json:"artifact"`
		}
		decode(t, w, &body)
		if body.Artifact.Version != 1 || body.Artifact.Content != "# v1" {
			t.Errorf("artifact = %+v", body.Artifact)
		}
	})

	t.Run("unknown kind is 400", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/c1/artifacts/plan", tok)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing artifact is 404", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/c1/artifacts/design", tok)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("non-integer version is 400", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/changes/c1/artifacts/proposal?version=x", tok)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestListTasksEndpoint(t *testing.T) {
	h := setupHandler(t)
	seedHandlerData(h.f)
	tok := h.token(t, "bob")

	w := h.do(t, http.MethodGet, "/changes/c1/tasks", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Tasks []taskDTO `json:"tasks"`
	}
	decode(t, w, &body)
	if len(body.Tasks) != 1 || body.Tasks[0].IssueID != "i2" ||
		body.Tasks[0].Title != "child" || body.Tasks[0].Stage != 1 {
		t.Errorf("tasks = %+v", body.Tasks)
	}
}

func TestNonMemberEndpointsForbidden(t *testing.T) {
	h := setupHandler(t)
	seedHandlerData(h.f)
	tok := h.token(t, "eve")

	for _, path := range []string{
		"/changes/c1", "/changes?issue_id=i1",
		"/changes/c1/artifacts", "/changes/c1/artifacts/proposal", "/changes/c1/tasks",
	} {
		if w := h.do(t, http.MethodGet, path, tok); w.Code != http.StatusForbidden {
			t.Errorf("GET %s status = %d, want 403", path, w.Code)
		}
	}
}
