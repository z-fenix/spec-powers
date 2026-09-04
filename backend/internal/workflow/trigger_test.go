package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

// splitFixture wires the read Service plus a working splitter over the
// recording fakes from splitter_test.go.
type splitFixture struct {
	handler   http.Handler
	tokens    *auth.TokenService
	svc       *Service
	changes   *splitChanges
	artifacts *splitArtifacts
	mappings  *splitMappings
	creator   *splitCreator
}

func setupSplitFixture(t *testing.T, withSplitter bool) *splitFixture {
	t.Helper()
	changes := &splitChanges{existingByIssue: map[string]*domain.Change{}}
	artifacts := &splitArtifacts{}
	mappings := &splitMappings{}
	creator := &splitCreator{}
	projects := &fakeProjects{existing: map[string]bool{"p1": true}, members: map[string]string{
		"p1|bob": "member",
	}}
	issues := &fakeIssues{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", ProjectID: "p1", Title: "父 issue", Description: "父描述"},
	}}
	svc := NewService(changes, artifacts, mappings, issues, projects)
	if withSplitter {
		svc = svc.WithSplitter(NewSplitter(SplitterDeps{
			Client:     stageOneLLM(tasksJSON),
			Changes:    changes,
			Artifacts:  artifacts,
			Mappings:   mappings,
			Issues:     issues,
			Creator:    creator,
			Contexts:   &splitContexts{},
			Templates:  defaultTemplates(),
			MaxRetries: 1,
		}))
	}
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(svc, tokens)
	r := chi.NewRouter()
	r.Route("/changes", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	return &splitFixture{handler: r, tokens: tokens, svc: svc, changes: changes, artifacts: artifacts, mappings: mappings, creator: creator}
}

func (f *splitFixture) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := f.tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (f *splitFixture) post(t *testing.T, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func TestStartSplitService(t *testing.T) {
	t.Run("member starts a split", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		change, tasks, err := f.svc.StartSplit(context.Background(), "bob", "i1")
		if err != nil {
			t.Fatalf("StartSplit: %v", err)
		}
		if change.ID != "c-new" || change.IssueID != "i1" || change.Phase != KindTasks {
			t.Errorf("change = %+v", change)
		}
		if len(tasks) != 2 || tasks[0].IssueID != "sub-1" || tasks[1].IssueID != "sub-2" {
			t.Errorf("tasks = %+v", tasks)
		}
		if len(f.creator.created) != 2 {
			t.Errorf("sub-issues created = %d, want 2", len(f.creator.created))
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		_, _, err := f.svc.StartSplit(context.Background(), "eve", "i1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != http.StatusForbidden {
			t.Fatalf("err = %v, want 403", err)
		}
		if len(f.creator.created) != 0 {
			t.Error("no sub-issues should be created for a forbidden user")
		}
	})

	t.Run("unknown issue is not found", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		_, _, err := f.svc.StartSplit(context.Background(), "bob", "nope")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != http.StatusNotFound {
			t.Fatalf("err = %v, want 404", err)
		}
	})

	t.Run("duplicate change conflicts", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		f.changes.existingByIssue["i1"] = &domain.Change{ID: "c-old", ProjectID: "p1", IssueID: "i1"}
		_, _, err := f.svc.StartSplit(context.Background(), "bob", "i1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != http.StatusConflict {
			t.Fatalf("err = %v, want 409", err)
		}
	})

	t.Run("splitter not configured is invalid", func(t *testing.T) {
		f := setupSplitFixture(t, false)
		_, _, err := f.svc.StartSplit(context.Background(), "bob", "i1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != http.StatusBadRequest {
			t.Fatalf("err = %v, want 400", err)
		}
	})
}

func TestStartSplitEndpoint(t *testing.T) {
	t.Run("creates change and returns tasks", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		tok := f.token(t, "bob")
		w := f.post(t, "/changes", tok, `{"issue_id":"i1"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Change changeDTO `json:"change"`
			Tasks  []taskDTO `json:"tasks"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Change.ID != "c-new" || body.Change.IssueID != "i1" || body.Change.Phase != "tasks" {
			t.Errorf("change = %+v", body.Change)
		}
		if len(body.Tasks) != 2 || body.Tasks[0].Title != "任务一" || body.Tasks[0].Stage != 1 {
			t.Errorf("tasks = %+v", body.Tasks)
		}
	})

	t.Run("requires auth", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		if w := f.post(t, "/changes", "", `{"issue_id":"i1"}`); w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("missing issue_id is invalid", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		tok := f.token(t, "bob")
		if w := f.post(t, "/changes", tok, `{}`); w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		tok := f.token(t, "eve")
		if w := f.post(t, "/changes", tok, `{"issue_id":"i1"}`); w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})

	t.Run("unknown issue is not found", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		tok := f.token(t, "bob")
		if w := f.post(t, "/changes", tok, `{"issue_id":"nope"}`); w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("duplicate change conflicts", func(t *testing.T) {
		f := setupSplitFixture(t, true)
		f.changes.existingByIssue["i1"] = &domain.Change{ID: "c-old", ProjectID: "p1", IssueID: "i1"}
		tok := f.token(t, "bob")
		if w := f.post(t, "/changes", tok, `{"issue_id":"i1"}`); w.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", w.Code)
		}
	})
}
