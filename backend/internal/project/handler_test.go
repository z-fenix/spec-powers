package project

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/auth"
)

func setupHandler(t *testing.T) (http.Handler, *TokenIssuer, *fakeUsers) {
	t.Helper()
	projects := newFakeProjects()
	users := newFakeUsers()
	svc := NewService(projects, users, newFakeMembers(), &fakeWorkspaces{})
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(svc, tokens, nil)
	return h.Routes(), &TokenIssuer{tokens: tokens}, users
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
	h, ti, _ := setupHandler(t)
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
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
	CreatedBy   string `json:"created_by"`
}

func TestProjectRoutesRequireAuth(t *testing.T) {
	h, _, _ := setupHandler(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/p1"},
		{http.MethodPatch, "/p1"},
		{http.MethodPost, "/p1/archive"},
		{http.MethodPost, "/p1/members"},
		{http.MethodGet, "/p1/resources"},
		{http.MethodPost, "/p1/resources"},
		{http.MethodDelete, "/p1/resources/r1"},
		{http.MethodGet, "/p1/context"},
		{http.MethodPut, "/p1/context"},
	} {
		w := authedRequest(t, h, tc.method, tc.path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: status = %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}

func TestListProjectsHandler(t *testing.T) {
	h, ti, _ := setupHandler(t)
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
	h, ti, _ := setupHandler(t)
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

	// Non-owner (unregistered caller) gets 403 before the user lookup.
	mateTok := ti.issue(t, "stranger")
	w = authedRequest(t, h, http.MethodPost, "/"+created.Project.ID+"/members", mateTok,
		map[string]string{"email": "ghost@x.com", "role": "member"})
	if w.Code != http.StatusForbidden {
		t.Errorf("non-owner status = %d, want 403", w.Code)
	}
}

func TestHandlerUnknownProjectIs404(t *testing.T) {
	h, ti, _ := setupHandler(t)
	tok := ti.issue(t, "u1")
	w := authedRequest(t, h, http.MethodPost, "/unknown/members", tok,
		map[string]string{"email": "ghost@x.com", "role": "member"})
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// setupProject creates a project owned by "u1", registers "mate@x.com" and
// adds them as a project member through the members endpoint. It returns the
// project id plus tokens for the owner and the member.
func setupProject(t *testing.T, h http.Handler, ti *TokenIssuer, users *fakeUsers) (projectID, ownerTok, mateTok string) {
	t.Helper()
	ownerTok = ti.issue(t, "u1")
	w := authedRequest(t, h, http.MethodPost, "/", ownerTok, map[string]string{"name": "Alpha", "description": "first"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: status = %d, body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Project domainProject `json:"project"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	mate, err := users.CreateUser(context.Background(), "mate@x.com", "h", "Mate")
	if err != nil {
		t.Fatalf("register mate: %v", err)
	}
	w = authedRequest(t, h, http.MethodPost, "/"+created.Project.ID+"/members", ownerTok,
		map[string]string{"email": "mate@x.com", "role": "member"})
	if w.Code != http.StatusCreated {
		t.Fatalf("add member: status = %d, body=%s", w.Code, w.Body.String())
	}
	return created.Project.ID, ownerTok, ti.issue(t, mate.ID)
}

func strangerToken(t *testing.T, ti *TokenIssuer) string {
	t.Helper()
	return ti.issue(t, "stranger")
}

func TestGetProjectHandler(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, mateTok := setupProject(t, h, ti, users)

	w := authedRequest(t, h, http.MethodGet, "/"+id, ownerTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("owner get: status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Project domainProject `json:"project"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Project.Name != "Alpha" || body.Project.Description != "first" || body.Project.Archived {
		t.Errorf("project = %+v", body.Project)
	}

	if w := authedRequest(t, h, http.MethodGet, "/"+id, mateTok, nil); w.Code != http.StatusOK {
		t.Errorf("member get: status = %d, want 200", w.Code)
	}

	if w := authedRequest(t, h, http.MethodGet, "/"+id, strangerToken(t, ti), nil); w.Code != http.StatusForbidden {
		t.Errorf("stranger get: status = %d, want 403", w.Code)
	}

	if w := authedRequest(t, h, http.MethodGet, "/unknown", ownerTok, nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown get: status = %d, want 404", w.Code)
	}
}

func TestUpdateProjectHandler(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, mateTok := setupProject(t, h, ti, users)

	w := authedRequest(t, h, http.MethodPatch, "/"+id, ownerTok,
		map[string]string{"name": "Beta", "description": "renamed"})
	if w.Code != http.StatusOK {
		t.Fatalf("owner patch: status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Beta") || !strings.Contains(w.Body.String(), "renamed") {
		t.Errorf("body = %s", w.Body.String())
	}

	w = authedRequest(t, h, http.MethodPatch, "/"+id, ownerTok,
		map[string]string{"name": " ", "description": "x"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty name patch: status = %d, want 400", w.Code)
	}

	w = authedRequest(t, h, http.MethodPatch, "/"+id, mateTok,
		map[string]string{"name": "X", "description": "y"})
	if w.Code != http.StatusForbidden {
		t.Errorf("member patch: status = %d, want 403", w.Code)
	}
}

func TestArchiveProjectHandler(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, mateTok := setupProject(t, h, ti, users)

	w := authedRequest(t, h, http.MethodPost, "/"+id+"/archive", ownerTok, map[string]bool{"archived": true})
	if w.Code != http.StatusOK {
		t.Fatalf("owner archive: status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"archived":true`) {
		t.Errorf("body = %s", w.Body.String())
	}

	w = authedRequest(t, h, http.MethodPost, "/"+id+"/archive", mateTok, map[string]bool{"archived": true})
	if w.Code != http.StatusForbidden {
		t.Errorf("member archive: status = %d, want 403", w.Code)
	}
}

func addResourceOK(t *testing.T, h http.Handler, id, tok string) {
	t.Helper()
	w := authedRequest(t, h, http.MethodPost, "/"+id+"/resources", tok,
		map[string]string{"type": "github_repo", "label": "main", "pointer": "octo/hello"})
	if w.Code != http.StatusCreated {
		t.Fatalf("add resource: status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestAddResourceHandlerValidatesPointer(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, _ := setupProject(t, h, ti, users)

	w := authedRequest(t, h, http.MethodPost, "/"+id+"/resources", ownerTok,
		map[string]string{"type": "github_repo", "label": "bad", "pointer": "not-a-repo"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid pointer: status = %d, want 400", w.Code)
	}

	w = authedRequest(t, h, http.MethodPost, "/"+id+"/resources", ownerTok,
		map[string]string{"type": "local_directory", "label": "bad", "pointer": "relative/path"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("relative dir: status = %d, want 400", w.Code)
	}

	addResourceOK(t, h, id, ownerTok)
}

func TestListResourcesHandler(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, mateTok := setupProject(t, h, ti, users)
	addResourceOK(t, h, id, ownerTok)

	w := authedRequest(t, h, http.MethodGet, "/"+id+"/resources", mateTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("member list: status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "octo/hello") {
		t.Errorf("body = %s", w.Body.String())
	}

	w = authedRequest(t, h, http.MethodGet, "/"+id+"/resources", strangerToken(t, ti), nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("stranger list: status = %d, want 403", w.Code)
	}
}

func TestRemoveResourceHandler(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, mateTok := setupProject(t, h, ti, users)
	addResourceOK(t, h, id, ownerTok)

	var listed struct {
		Resources []struct {
			ID string `json:"id"`
		} `json:"resources"`
	}
	w := authedRequest(t, h, http.MethodGet, "/"+id+"/resources", ownerTok, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed.Resources) != 1 {
		t.Fatalf("resources = %+v", listed.Resources)
	}
	resourceID := listed.Resources[0].ID

	w = authedRequest(t, h, http.MethodDelete, "/"+id+"/resources/"+resourceID, mateTok, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("member delete: status = %d, want 403", w.Code)
	}

	w = authedRequest(t, h, http.MethodDelete, "/"+id+"/resources/r-missing", ownerTok, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("delete missing: status = %d, want 404", w.Code)
	}

	w = authedRequest(t, h, http.MethodDelete, "/"+id+"/resources/"+resourceID, ownerTok, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("owner delete: status = %d, want 204", w.Code)
	}
}

func TestContextHandlers(t *testing.T) {
	h, ti, users := setupHandler(t)
	id, ownerTok, mateTok := setupProject(t, h, ti, users)

	w := authedRequest(t, h, http.MethodGet, "/"+id+"/context", mateTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("member read empty context: status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"content":""`) {
		t.Errorf("body = %s", w.Body.String())
	}

	w = authedRequest(t, h, http.MethodGet, "/"+id+"/context", strangerToken(t, ti), nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("stranger read: status = %d, want 403", w.Code)
	}

	w = authedRequest(t, h, http.MethodPut, "/"+id+"/context", mateTok, map[string]string{"content": "nope"})
	if w.Code != http.StatusForbidden {
		t.Errorf("member write: status = %d, want 403", w.Code)
	}

	w = authedRequest(t, h, http.MethodPut, "/"+id+"/context", ownerTok, map[string]string{"content": "notes"})
	if w.Code != http.StatusOK {
		t.Fatalf("owner write: status = %d, body=%s", w.Code, w.Body.String())
	}

	w = authedRequest(t, h, http.MethodGet, "/"+id+"/context", mateTok, nil)
	if !strings.Contains(w.Body.String(), "notes") {
		t.Errorf("body = %s", w.Body.String())
	}
}
