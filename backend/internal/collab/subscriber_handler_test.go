package collab

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
)

func TestSubscriberRoutes(t *testing.T) {
	f := newSubscriberFixture(t)
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(f.svc, tokens)
	// mirror the production mount: collab lives under
	// /projects/{projectID}/issues/{issueID}
	r := chi.NewRouter()
	r.Route("/{projectID}/issues/{issueID}", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	hf := &handlerFixture{handler: r, tokens: tokens, svc: f.svc}

	tok := func(userID string) string {
		t.Helper()
		token, err := tokens.Issue(userID)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		return token
	}

	alice := tok("alice")

	t.Run("add and list via HTTP", func(t *testing.T) {
		w := hf.do(t, http.MethodPost, "/p1/issues/i1/subscribers", alice, map[string]string{"email": "bob@example.com"})
		if w.Code != http.StatusCreated {
			t.Fatalf("add status = %d, body = %s", w.Code, w.Body.String())
		}
		var addRes struct {
			Subscribers []struct {
				UserID string `json:"user_id"`
			} `json:"subscribers"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &addRes); err != nil {
			t.Fatalf("decode add: %v", err)
		}
		if len(addRes.Subscribers) != 1 || addRes.Subscribers[0].UserID != "bob" {
			t.Fatalf("add response = %s", w.Body.String())
		}

		w = hf.do(t, http.MethodGet, "/p1/issues/i1/subscribers", alice, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("list status = %d", w.Code)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &addRes); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(addRes.Subscribers) != 1 || addRes.Subscribers[0].UserID != "bob" {
			t.Fatalf("list response = %s", w.Body.String())
		}
	})

	t.Run("remove via HTTP", func(t *testing.T) {
		w := hf.do(t, http.MethodDelete, "/p1/issues/i1/subscribers/bob", alice, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("remove status = %d, body = %s", w.Code, w.Body.String())
		}
		w = hf.do(t, http.MethodDelete, "/p1/issues/i1/subscribers/bob", alice, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("repeat remove status = %d, want 404", w.Code)
		}
	})

	t.Run("unauthenticated requests are rejected", func(t *testing.T) {
		w := hf.do(t, http.MethodGet, "/p1/issues/i1/subscribers", "", nil)
		if w.Code == http.StatusOK {
			t.Fatalf("status = %d, want unauthorized", w.Code)
		}
	})
}
