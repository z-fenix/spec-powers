package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("path = %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["email"] != "a@b.c" || req["password"] != "pw" {
			t.Errorf("bad credentials: %v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"token": "tok-1",
			"user":  map[string]any{"id": "u1", "email": "a@b.c", "display_name": "A"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	res, err := c.Login("a@b.c", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Token != "tok-1" || res.User.ID != "u1" || res.User.Email != "a@b.c" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestClientErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "conflict", "message": "gate failed: no handoff recorded"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetChange("c1")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != http.StatusConflict || apiErr.Code != "conflict" ||
		apiErr.Message != "gate failed: no handoff recorded" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestClientAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"change": map[string]any{"id": "c1"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-7")
	if _, err := c.GetChange("c1"); err != nil {
		t.Fatalf("GetChange: %v", err)
	}
	if gotAuth != "Bearer tok-7" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestClientGetChangeByIssue(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/changes" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		query = r.URL.Query().Get("issue_id")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"change": map[string]any{
				"id": "c9", "project_id": "p1", "issue_id": "i2", "phase": "proposal", "status": "active",
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	change, err := c.GetChangeByIssue("i2")
	if err != nil {
		t.Fatalf("GetChangeByIssue: %v", err)
	}
	if query != "i2" {
		t.Fatalf("issue_id query = %q", query)
	}
	if change.ID != "c9" || change.Phase != "proposal" || change.Status != "active" {
		t.Fatalf("unexpected change: %+v", change)
	}
}

func TestClientGuardAdvanceVerifyArchive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/changes/c1/guard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"guard": map[string]any{
				"change_id": "c1", "phase": "tasks", "next_phase": "",
				"phase_legal": true, "handoff_fresh": true, "verify_passed": true,
				"can_advance": false, "can_archive": true,
				"reasons": []string{},
			}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"change":  map[string]any{"id": "c1", "phase": "design", "status": "active"},
			"handoff": map[string]any{"id": "h1", "from_phase": "design", "to_phase": "tasks"},
		})
	})
	mux.HandleFunc("/api/v1/changes/c1/verify", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"artifact": map[string]any{"id": "a1", "kind": "verify", "version": 1},
			"result":   "pass", "passed": true,
		})
	})
	mux.HandleFunc("/api/v1/changes/c1/archive", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"change": map[string]any{"id": "c1", "phase": "tasks", "status": "archived"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "tok")

	g, err := c.GetGuard("c1")
	if err != nil {
		t.Fatalf("GetGuard: %v", err)
	}
	if g.Phase != "tasks" || !g.CanArchive || g.CanAdvance {
		t.Fatalf("unexpected guard: %+v", g)
	}

	change, handoff, err := c.AdvanceGuard("c1")
	if err != nil {
		t.Fatalf("AdvanceGuard: %v", err)
	}
	if change.Phase != "design" || handoff.FromPhase != "design" || handoff.ToPhase != "tasks" {
		t.Fatalf("unexpected advance: %+v %+v", change, handoff)
	}

	result, passed, err := c.SubmitVerify("c1", "result: pass\n")
	if err != nil {
		t.Fatalf("SubmitVerify: %v", err)
	}
	if result != "pass" || !passed {
		t.Fatalf("unexpected verify: %s %v", result, passed)
	}

	change, err = c.Archive("c1")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if change.Status != "archived" {
		t.Fatalf("unexpected status: %s", change.Status)
	}
}
