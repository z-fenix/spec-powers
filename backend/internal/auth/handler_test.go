package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setupHandler(t *testing.T) (http.Handler, *TokenService, *fakeUserStore) {
	t.Helper()
	users := newFakeUsers()
	svc := NewService(users, &fakeWorkspaceStore{}, &fakeMemberStore{}, NewTokenService("test-secret", 15*time.Minute))
	return NewHandler(svc).Routes(), svc.tokens, users
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

type userBody struct {
	Token string `json:"token"`
	User struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	} `json:"user"`
}

func TestRegisterHandler(t *testing.T) {
	h, _, _ := setupHandler(t)
	w := postJSON(t, h, "/register", map[string]string{
		"email": "eve@example.com", "password": "password123", "display_name": "Eve",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body userBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User.Email != "eve@example.com" || body.User.ID == "" || body.User.DisplayName != "Eve" {
		t.Errorf("user = %+v", body.User)
	}
	// Register must also issue a token so clients (sp login --register)
	// are authenticated right away, mirroring /auth/login.
	if body.Token == "" {
		t.Error("register response missing token")
	}
}

func TestRegisterHandlerValidationErrors(t *testing.T) {
	h, _, _ := setupHandler(t)
	cases := []struct {
		body   map[string]string
		status int
		code   string
	}{
		{map[string]string{"email": "bad", "password": "password123", "display_name": "E"}, 400, "invalid_request"},
		{map[string]string{"email": "a@b.com", "password": "short", "display_name": "E"}, 400, "invalid_request"},
	}
	for _, c := range cases {
		w := postJSON(t, h, "/register", c.body)
		if w.Code != c.status {
			t.Errorf("body %v: status = %d, want %d", c.body, w.Code, c.status)
		}
		if !strings.Contains(w.Body.String(), `"code":"`+c.code+`"`) {
			t.Errorf("body %v: envelope = %s", c.body, w.Body.String())
		}
	}
}

func TestRegisterHandlerMalformedJSON(t *testing.T) {
	h, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("{not json"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestLoginHandler(t *testing.T) {
	h, _, _ := setupHandler(t)
	if w := postJSON(t, h, "/register", map[string]string{
		"email": "finn@example.com", "password": "password123", "display_name": "Finn",
	}); w.Code != http.StatusCreated {
		t.Fatalf("register status = %d", w.Code)
	}

	w := postJSON(t, h, "/login", map[string]string{"email": "finn@example.com", "password": "password123"})
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Token string   `json:"token"`
		User  userBody `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Token == "" {
		t.Error("empty token")
	}

	// Wrong password → 401 envelope.
	w = postJSON(t, h, "/login", map[string]string{"email": "finn@example.com", "password": "wrong-pass"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password status = %d, want 401", w.Code)
	}
}

func TestMeHandlerRequiresAuth(t *testing.T) {
	h, _, _ := setupHandler(t)

	// No token → 401.
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", w.Code)
	}

	// Register and log in.
	postJSON(t, h, "/register", map[string]string{
		"email": "gina@example.com", "password": "password123", "display_name": "Gina",
	})
	w = postJSON(t, h, "/login", map[string]string{"email": "gina@example.com", "password": "password123"})
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil || login.Token == "" {
		t.Fatalf("login response: %s", w.Body.String())
	}

	// Good token → 200 with user.
	req = httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("me status = %d, body=%s", w.Code, w.Body.String())
	}
	var me struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || me.User.Email != "gina@example.com" {
		t.Errorf("me response = %s", w.Body.String())
	}

	// Garbage token → 401.
	req = httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("bad token status = %d, want 401", w.Code)
	}
}
