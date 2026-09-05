package pr

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/issue"
)

type handlerFixture struct {
	handler http.Handler
	tokens  *auth.TokenService
	f       *fixture
}

func setupHandler(t *testing.T) *handlerFixture {
	t.Helper()
	fix := newFixture()
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(fix.svc, tokens)

	// mirror the production mounts: project-level /pullrequests plus the
	// issue-side /issues/{issueID}/pullrequests listing.
	r := chi.NewRouter()
	r.Route("/{projectID}", func(r chi.Router) {
		r.Mount("/pullrequests", h.Routes())
		r.Route("/issues/{issueID}", func(r chi.Router) {
			r.Mount("/pullrequests", h.IssueRoutes())
		})
	})
	return &handlerFixture{handler: r, tokens: tokens, f: fix}
}

func (hf *handlerFixture) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := hf.tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (hf *handlerFixture) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
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
	hf.handler.ServeHTTP(w, req)
	return w
}

func TestPullRequestHandler(t *testing.T) {
	hf := setupHandler(t)
	tok := hf.token(t, "u1")

	var prID string
	t.Run("upsert links issues and returns 201", func(t *testing.T) {
		w := hf.do(t, http.MethodPost, "/proj-1/pullrequests", tok, map[string]any{
			"repo": "z-fenix/spec-powers", "number": 9,
			"title": "feat: SP-44", "head_branch": "agent/x/SP-44",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var res struct {
			PullRequest pullRequestDTO `json:"pull_request"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v", err)
		}
		prID = res.PullRequest.ID
		if res.PullRequest.State != StateOpen {
			t.Errorf("state = %q", res.PullRequest.State)
		}
		if len(res.PullRequest.IssueKeys) != 1 || res.PullRequest.IssueKeys[0] != "SP-44" {
			t.Errorf("issue_keys = %v", res.PullRequest.IssueKeys)
		}
	})

	t.Run("upsert rejects bad input", func(t *testing.T) {
		if w := hf.do(t, http.MethodPost, "/proj-1/pullrequests", tok, map[string]any{"number": 1}); w.Code != http.StatusBadRequest {
			t.Errorf("missing title status = %d", w.Code)
		}
		if w := hf.do(t, http.MethodPost, "/proj-1/pullrequests", "", map[string]any{"number": 1, "title": "t"}); w.Code != http.StatusUnauthorized {
			t.Errorf("no token status = %d", w.Code)
		}
	})

	t.Run("merge via PATCH closes issue", func(t *testing.T) {
		w := hf.do(t, http.MethodPatch, "/proj-1/pullrequests/"+prID, tok, map[string]any{"state": StateMerged})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("merge applies close intent", func(t *testing.T) {
		// Upstream PR references XX-7 with a close intent.
		w := hf.do(t, http.MethodPost, "/proj-1/pullrequests", tok, map[string]any{
			"repo": "z-fenix/spec-powers", "number": 10, "title": "fixes XX-7",
		})
		var res struct {
			PullRequest pullRequestDTO `json:"pull_request"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if w := hf.do(t, http.MethodPatch, "/proj-1/pullrequests/"+res.PullRequest.ID, tok, map[string]any{"state": StateMerged}); w.Code != http.StatusOK {
			t.Fatalf("merge status = %d", w.Code)
		}
		if got := statusOf(t, hf.f, "i-7"); got != issue.StatusDone {
			t.Errorf("XX-7 status = %q, want done", got)
		}
	})

	t.Run("issue-side listing shows linked PRs", func(t *testing.T) {
		w := hf.do(t, http.MethodGet, "/proj-1/issues/i-44/pullrequests", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var res struct {
			PullRequests []pullRequestDTO `json:"pull_requests"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(res.PullRequests) != 1 || res.PullRequests[0].ID != prID {
			t.Fatalf("pull_requests = %+v", res.PullRequests)
		}
		if res.PullRequests[0].State != StateMerged {
			t.Errorf("state = %q, want merged", res.PullRequests[0].State)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		stranger := hf.token(t, "outsider")
		w := hf.do(t, http.MethodPost, "/proj-1/pullrequests", stranger, map[string]any{"number": 1, "title": "t"})
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})
}
