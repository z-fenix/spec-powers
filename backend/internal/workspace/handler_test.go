package workspace

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
)

func setupHandler(t *testing.T) (http.Handler, *auth.TokenService, *fixture) {
	t.Helper()
	f := newFixture(t)
	tokens := auth.NewTokenService("workspace-test-secret", 15*time.Minute)
	r := chi.NewRouter()
	r.Mount("/api/v1/workspace", NewHandler(f.svc, tokens).Routes())
	return r, tokens, f
}

func doJSON(t *testing.T, h http.Handler, tokens *auth.TokenService, method, path, userID string, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
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
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var res map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return w.Code, res
}

func TestHandlerMembersRequiresAuth(t *testing.T) {
	h, _, _ := setupHandler(t)
	code, _ := doJSON(t, h, nil, http.MethodGet, "/api/v1/workspace/members", "", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", code)
	}
}

func TestHandlerMembersList(t *testing.T) {
	h, tokens, f := setupHandler(t)
	code, body := doJSON(t, h, tokens, http.MethodGet, "/api/v1/workspace/members", f.ownerID, "")
	if code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
	if body["viewer_role"] != "owner" {
		t.Fatalf("viewer_role = %v", body["viewer_role"])
	}
	members := body["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2", len(members))
	}
	first := members[0].(map[string]any)
	for _, key := range []string{"user_id", "email", "display_name", "role", "joined_at"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("member missing key %q: %v", key, first)
		}
	}
}

func TestHandlerInviteFlow(t *testing.T) {
	h, tokens, f := setupHandler(t)

	// Unknown email → pending invite with a code.
	code, body := doJSON(t, h, tokens, http.MethodPost, "/api/v1/workspace/members/invite", f.ownerID,
		`{"email":"ghost@example.com","role":"member"}`)
	if code != http.StatusCreated {
		t.Fatalf("got %d, want 201", code)
	}
	if body["joined"] != false {
		t.Fatalf("joined = %v, want false", body["joined"])
	}
	inviteCode, _ := body["code"].(string)
	if inviteCode == "" {
		t.Fatalf("missing invite code: %v", body)
	}

	// Redeem as the invited user after registering.
	f.users.CreateUser(t.Context(), "ghost@example.com", "", "Ghost")
	ghostID := "u-ghost@example.com"
	code, _ = doJSON(t, h, tokens, http.MethodPost, "/api/v1/workspace/invites/redeem", ghostID,
		`{"code":"`+inviteCode+`"}`)
	if code != http.StatusOK {
		t.Fatalf("redeem got %d: %v", code, body)
	}

	// Registered user → direct join.
	f.users.CreateUser(t.Context(), "fresh@example.com", "", "Fresh")
	code, body = doJSON(t, h, tokens, http.MethodPost, "/api/v1/workspace/members/invite", f.ownerID,
		`{"email":"fresh@example.com","role":"member"}`)
	if code != http.StatusCreated {
		t.Fatalf("got %d, want 201", code)
	}
	if body["joined"] != true {
		t.Fatalf("joined = %v, want true", body["joined"])
	}
}

func TestHandlerInviteDirectJoin(t *testing.T) {
	h, tokens, f := setupHandler(t)
	f.users.CreateUser(t.Context(), "new@example.com", "", "New")
	code, body := doJSON(t, h, tokens, http.MethodPost, "/api/v1/workspace/members/invite", f.ownerID,
		`{"email":"new@example.com","role":"member"}`)
	if code != http.StatusCreated {
		t.Fatalf("got %d, want 201", code)
	}
	if body["joined"] != true {
		t.Fatalf("joined = %v, want true", body["joined"])
	}
	if body["member"] == nil {
		t.Fatalf("missing member: %v", body)
	}
}

func TestHandlerInviteForbiddenForNonOwner(t *testing.T) {
	h, tokens, f := setupHandler(t)
	code, _ := doJSON(t, h, tokens, http.MethodPost, "/api/v1/workspace/members/invite", f.memberID,
		`{"email":"ghost@example.com","role":"member"}`)
	if code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", code)
	}
}

func TestHandlerSetRole(t *testing.T) {
	h, tokens, f := setupHandler(t)
	code, body := doJSON(t, h, tokens, http.MethodPatch, "/api/v1/workspace/members/"+f.memberID, f.ownerID,
		`{"role":"owner"}`)
	if code != http.StatusOK {
		t.Fatalf("got %d: %v", code, body)
	}
	member := body["member"].(map[string]any)
	if member["role"] != "owner" {
		t.Fatalf("role = %v, want owner", member["role"])
	}
}

func TestHandlerSetRoleValidatesBody(t *testing.T) {
	h, tokens, f := setupHandler(t)
	code, _ := doJSON(t, h, tokens, http.MethodPatch, "/api/v1/workspace/members/"+f.memberID, f.ownerID,
		`{"role":"admin"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
}

func TestHandlerListAndRevokeInvites(t *testing.T) {
	h, tokens, f := setupHandler(t)
	doJSON(t, h, tokens, http.MethodPost, "/api/v1/workspace/members/invite", f.ownerID,
		`{"email":"ghost@example.com","role":"member"}`)

	code, body := doJSON(t, h, tokens, http.MethodGet, "/api/v1/workspace/invites", f.ownerID, "")
	if code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
	invites := body["invites"].([]any)
	if len(invites) != 1 {
		t.Fatalf("got %d invites, want 1", len(invites))
	}
	invite := invites[0].(map[string]any)
	id, _ := invite["id"].(string)

	code, body = doJSON(t, h, tokens, http.MethodDelete, "/api/v1/workspace/invites/"+id, f.ownerID, "")
	if code != http.StatusOK {
		t.Fatalf("revoke got %d: %v", code, body)
	}

	code, body = doJSON(t, h, tokens, http.MethodGet, "/api/v1/workspace/invites", f.ownerID, "")
	if invites := body["invites"].([]any); len(invites) != 0 {
		t.Fatalf("got %d invites after revoke, want 0", len(invites))
	}
}
