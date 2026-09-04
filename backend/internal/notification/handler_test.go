package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
)

func setupHandler(t *testing.T) http.Handler {
	t.Helper()
	f := &fakeStore{}
	svc := NewService(f)
	tokens := auth.NewTokenService("notification-test-secret", 15*time.Minute)
	r := chi.NewRouter()
	r.Mount("/api/v1/notifications", NewHandler(svc, tokens).Routes())
	return r
}

func token(t *testing.T, tokens *auth.TokenService, userID string) string {
	t.Helper()
	tok, err := tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func doRequest(t *testing.T, h http.Handler, tokens *auth.TokenService, method, path, userID string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+token(t, tokens, userID))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return w.Code, body
}

func TestHandlerList(t *testing.T) {
	f := &fakeStore{}
	svc := NewService(f)
	svc.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "first", Body: "b1", IssueID: "i1"})
	svc.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "run_finished", Title: "second", IssueID: "i2"})
	svc.Notify(context.Background(), NotifyInput{UserID: "u2", Kind: "comment", Title: "foreign"})

	tokens := auth.NewTokenService("notification-test-secret", 15*time.Minute)
	r := chi.NewRouter()
	r.Mount("/api/v1/notifications", NewHandler(svc, tokens).Routes())

	code, body := doRequest(t, r, tokens, http.MethodGet, "/api/v1/notifications", "u1")
	if code != http.StatusOK {
		t.Fatalf("list: got %d", code)
	}
	list := body["notifications"].([]any)
	if len(list) != 2 {
		t.Fatalf("got %d notifications, want 2", len(list))
	}
	first := list[0].(map[string]any)
	if first["title"] != "second" {
		t.Fatalf("newest first violated: %v", first)
	}
	if first["read"] != false {
		t.Fatalf("new notification must be unread: %v", first)
	}
	if body["unread"].(float64) != 2 {
		t.Fatalf("unread count: %v", body["unread"])
	}
	if first["created_at"] == "" {
		t.Fatal("created_at missing")
	}

	id := first["id"].(string)
	code, body = doRequest(t, r, tokens, http.MethodPost, "/api/v1/notifications/"+id+"/read", "u1")
	if code != http.StatusOK {
		t.Fatalf("mark read: got %d", code)
	}
	if body["notification"].(map[string]any)["read"] != true {
		t.Fatalf("notification not marked read: %v", body)
	}

	code, body = doRequest(t, r, tokens, http.MethodGet, "/api/v1/notifications?unread=true", "u1")
	if code != http.StatusOK {
		t.Fatalf("unread list: got %d", code)
	}
	if len(body["notifications"].([]any)) != 1 {
		t.Fatalf("unread list should hold one row: %v", body)
	}
}

func TestHandlerMarkAllRead(t *testing.T) {
	f := &fakeStore{}
	svc := NewService(f)
	svc.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "a"})
	svc.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "b"})

	tokens := auth.NewTokenService("notification-test-secret", 15*time.Minute)
	r := chi.NewRouter()
	r.Mount("/api/v1/notifications", NewHandler(svc, tokens).Routes())

	code, body := doRequest(t, r, tokens, http.MethodPost, "/api/v1/notifications/read-all", "u1")
	if code != http.StatusOK || body["marked"].(float64) != 2 {
		t.Fatalf("mark all read: %d %v", code, body)
	}
	_, body = doRequest(t, r, tokens, http.MethodGet, "/api/v1/notifications", "u1")
	if body["unread"].(float64) != 0 {
		t.Fatalf("unread after read-all: %v", body["unread"])
	}
}

func TestHandlerOwnership(t *testing.T) {
	f := &fakeStore{}
	svc := NewService(f)
	svc.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "mine"})

	tokens := auth.NewTokenService("notification-test-secret", 15*time.Minute)
	r := chi.NewRouter()
	r.Mount("/api/v1/notifications", NewHandler(svc, tokens).Routes())

	code, _ := doRequest(t, r, tokens, http.MethodPost, "/api/v1/notifications/"+f.created[0].ID+"/read", "u2")
	if code != http.StatusNotFound {
		t.Fatalf("foreign mark read: got %d", code)
	}
}

func TestHandlerRequiresAuth(t *testing.T) {
	setupHandler(t)
	f := &fakeStore{}
	svc := NewService(f)
	tk := auth.NewTokenService("notification-test-secret", 15*time.Minute)
	srv := httptest.NewServer(NewHandler(svc, tk).Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: got %d: %s", resp.StatusCode, body)
	}
}
