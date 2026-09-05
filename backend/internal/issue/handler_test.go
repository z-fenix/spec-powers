package issue

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
)

type handlerFixture struct {
	handler http.Handler
	tokens  *auth.TokenService
	svc     *Service
	events  *fakeEventStore
}

func setupHandler(t *testing.T) *handlerFixture {
	t.Helper()
	projects := &fakeProjects{
		existing: map[string]bool{"p1": true},
		members:  map[string]string{"p1|alice": "owner", "p1|bob": "member"},
	}
	users := &fakeUsers{ids: map[string]bool{"carol": true}}
	events := &fakeEventStore{}
	svc := NewService(newFakeIssues(), projects, users).WithEventStore(events)
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(svc, tokens)

	// mirror the production mount: issues live under /projects/{projectID}/issues
	r := chi.NewRouter()
	r.Route("/{projectID}", func(r chi.Router) {
		r.Mount("/issues", h.Routes())
	})
	return &handlerFixture{handler: r, tokens: tokens, svc: svc, events: events}
}

func (f *handlerFixture) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := f.tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (f *handlerFixture) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func TestIssueHandlerCRUD(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	t.Run("create returns 201 with issue", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues", tok, map[string]any{
			"title": "Ship issue domain", "priority": "high", "labels": []string{"backend"},
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Issue.Title != "Ship issue domain" || body.Issue.Status != "todo" || body.Issue.Priority != "high" {
			t.Errorf("issue = %+v", body.Issue)
		}
	})

	t.Run("create with blank title is 400", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues", tok, map[string]any{"title": "  "})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("malformed body is 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/p1/issues", bytes.NewReader([]byte("{oops")))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	created := ""
	t.Run("get returns the issue", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues", tok, map[string]any{"title": "read me"})
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		created = body.Issue.ID

		w = f.do(t, http.MethodGet, "/p1/issues/"+created, tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var got struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if got.Issue.ID != created || got.Issue.Title != "read me" {
			t.Errorf("issue = %+v", got.Issue)
		}
	})

	t.Run("patch updates fields", func(t *testing.T) {
		w := f.do(t, http.MethodPatch, "/p1/issues/"+created, tok, map[string]any{
			"title": "renamed", "assignee_id": "carol",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Issue.Title != "renamed" || body.Issue.AssigneeID != "carol" {
			t.Errorf("issue = %+v", body.Issue)
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/missing", tok, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("delete returns 204", func(t *testing.T) {
		w := f.do(t, http.MethodDelete, "/p1/issues/"+created, tok, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d", w.Code)
		}
		w = f.do(t, http.MethodGet, "/p1/issues/"+created, tok, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("deleted issue still readable: %d", w.Code)
		}
	})
}

func TestIssueHandlerStatusAndChildren(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	mkIssue := func(title string, parentID string, stage int) issueDTO {
		t.Helper()
		payload := map[string]any{"title": title, "stage": stage}
		if parentID != "" {
			payload["parent_id"] = parentID
		}
		w := f.do(t, http.MethodPost, "/p1/issues", tok, payload)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d body=%s", title, w.Code, w.Body.String())
		}
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body.Issue
	}

	parent := mkIssue("parent", "", 0)
	c2 := mkIssue("c2", parent.ID, 2)
	c0 := mkIssue("c0", parent.ID, 0)

	t.Run("status endpoint transitions", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues/"+c0.ID+"/status", tok, map[string]string{"status": "in_progress"})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Issue.Status != "in_progress" {
			t.Errorf("status = %q, want in_progress", body.Issue.Status)
		}
	})

	t.Run("illegal transition is 400", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues/"+c0.ID+"/status", tok, map[string]string{"status": "done"})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("children are ordered by stage then position", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/"+parent.ID+"/children", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var body struct {
			Issues []issueDTO `json:"issues"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Issues) != 2 {
			t.Fatalf("len = %d, want 2", len(body.Issues))
		}
		if body.Issues[0].ID != c0.ID || body.Issues[1].ID != c2.ID {
			t.Errorf("order = %s,%s; want %s,%s", body.Issues[0].ID, body.Issues[1].ID, c0.ID, c2.ID)
		}
	})
}

func TestIssueHandlerListFilters(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	mk := func(title string, parentID string, status string) issueDTO {
		t.Helper()
		payload := map[string]any{"title": title}
		if parentID != "" {
			payload["parent_id"] = parentID
		}
		w := f.do(t, http.MethodPost, "/p1/issues", tok, payload)
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if status != "todo" {
			w = f.do(t, http.MethodPost, "/p1/issues/"+body.Issue.ID+"/status", tok, map[string]string{"status": status})
			if w.Code != http.StatusOK {
				t.Fatalf("transition %s: %d", status, w.Code)
			}
		}
		return body.Issue
	}

	root := mk("root", "", "todo")
	kid := mk("kid", root.ID, "todo")
	mk("inflight", "", "in_progress")

	t.Run("status filter", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues?status=in_progress", tok, nil)
		var body struct {
			Issues []issueDTO `json:"issues"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Issues) != 1 || body.Issues[0].Status != "in_progress" {
			t.Errorf("issues = %+v", body.Issues)
		}
	})

	t.Run("parent=root filter", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues?parent=root", tok, nil)
		var body struct {
			Issues []issueDTO `json:"issues"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		for _, i := range body.Issues {
			if i.ID == kid.ID {
				t.Errorf("child leaked into root filter: %+v", i)
			}
		}
		if len(body.Issues) != 2 {
			t.Errorf("len = %d, want 2", len(body.Issues))
		}
	})

	t.Run("bad stage filter is 400", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues?stage=abc", tok, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestIssueHandlerAuthAndPermission(t *testing.T) {
	f := setupHandler(t)

	t.Run("routes require auth", func(t *testing.T) {
		for _, tc := range []struct{ method, path string }{
			{http.MethodPost, "/p1/issues"},
			{http.MethodGet, "/p1/issues"},
			{http.MethodGet, "/p1/issues/x"},
			{http.MethodPatch, "/p1/issues/x"},
			{http.MethodDelete, "/p1/issues/x"},
			{http.MethodPost, "/p1/issues/x/status"},
			{http.MethodGet, "/p1/issues/x/children"},
		} {
			w := f.do(t, tc.method, tc.path, "", nil)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: status = %d, want 401", tc.method, tc.path, w.Code)
			}
		}
	})

	t.Run("stranger gets 403", func(t *testing.T) {
		tok := f.token(t, "mallory")
		w := f.do(t, http.MethodGet, "/p1/issues", tok, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})
}
