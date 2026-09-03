package project

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"


	"specpowers/backend/internal/auth"
)

func setupHandler(t *testing.T) (http.Handler, *TokenIssuer) {
	t.Helper()
	projects := newFakeProjects()
	users := newFakeUsers()
	svc := NewService(projects, users, newFakeMembers(), &fakeWorkspaces{})
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(svc, tokens)
	return h.Routes(), &TokenIssuer{tokens: tokens}
}

type TokenIssuer struct {
	tokens *auth.TokenService
}

func (ti *TokenIssuer) issue(t *testing.T, userID string) string {
	t.Helper()
	tok, err := ti.tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return tok
}

func authedRequest(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
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
	h.ServeHTTP(w, req)
	return w
}

func TestCreateProjectHandler(t *testing.T) {
	h, ti := setupHandler(t)
	tok := ti.issue(t, "u1")
	w := authedRequest(t, h, http.MethodPost, "/", tok, map[string]string{"name": "Alpha"})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Project domainProject `json:"project"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Project.Name != "Alpha" || body.Project.ID == "" {
		t.Errorf("project = %+v", body.Project)
	}
}

type domainProject struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	CreatedBy   string `json:"created_by"`
}

func TestProjectRoutesRequireAuth(t *testing.T) {
	h, _ := setupHandler(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/"},
		{http.MethodGet, "/"},
		{http.MethodPost, "/p1/members"},
	} {
		w := authedRequest(t, h, tc.method, tc.path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: status = %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}

func TestListProjectsHandler(t *testing.T) {
	h, ti := setupHandler(t)
	tok := ti.issue(t, "u1")
	authedRequest(t, h, http.MethodPost, "/", tok, map[string]string{"name": "Alpha"})

	w := authedRequest(t, h, http.MethodGet, "/", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Alpha") {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestAddMemberHandler(t *testing.T) {
	h, ti := setupHandler(t)
	ownerTok := ti.issue(t, "u1")
	w := authedRequest(t, h, http.MethodPost, "/", ownerTok, map[string]string{"name": "Alpha"})
	var created struct {
		Project domainProject `json:"project"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Unknown target user → 404.
	w = authedRequest(t, h, http.MethodPost, "/"+created.Project.ID+"/members", ownerTok,
		map[string]string{"email": "ghost@x.com", "role": "member"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown user status = %d, body=%s", w.Code, w.Body.String())
	}

	// Register a real user through the project service's user store is not
	// exposed here; instead verify owner check: non-owner gets 403.
	mateTok := ti.issue(t, "stranger")
	w = authedRequest(t, h, http.MethodPost, "/"+created.Project.ID+"/members", mateTok,
		map[string]string{"email": "ghost@x.com", "role": "member"})
	if w.Code != http.StatusForbidden {
		t.Errorf("non-owner status = %d, want 403", w.Code)
	}
}

func TestHandlerUnknownProjectIs404(t *testing.T) {
	h, ti := setupHandler(t)
	// Register user via service then request member add on unknown project.
	tok := ti.issue(t, "u1")
	w := authedRequest(t, h, http.MethodPost, "/unknown/members", tok,
		map[string]string{"email": "ghost@x.com", "role": "member"})
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

