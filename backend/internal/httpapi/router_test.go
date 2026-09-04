package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppErrorEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, ErrNotFound("thing missing"))

	res := w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" || body.Error.Message != "thing missing" {
		t.Errorf("envelope = %+v", body)
	}
}

func TestAppErrorHelpers(t *testing.T) {
	cases := []struct {
		err    *AppError
		status int
		code   string
	}{
		{ErrInvalid("bad"), 400, "invalid_request"},
		{ErrUnauthorized("no"), 401, "unauthorized"},
		{ErrForbidden("no"), 403, "forbidden"},
		{ErrNotFound("no"), 404, "not_found"},
		{ErrConflict("dup"), 409, "conflict"},
		{ErrInternal("boom"), 500, "internal"},
	}
	for _, c := range cases {
		if c.err.Status != c.status || c.err.Code != c.code {
			t.Errorf("%s: got (%d,%s), want (%d,%s)", c.code, c.err.Status, c.err.Code, c.status, c.code)
		}
	}
}

func TestRouterHealth(t *testing.T) {
	h := NewRouter(Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

func TestRouterUnknownRouteUsesEnvelope(t *testing.T) {
	h := NewRouter(Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", body.Error.Code)
	}
}

func TestRouterMountsChanges(t *testing.T) {
	reached := false
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if r.URL.Path != "/c1" {
			t.Errorf("stripped path = %q, want /c1", r.URL.Path)
		}
	})
	h := NewRouter(Deps{Changes: stub})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/changes/c1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !reached {
		t.Error("changes handler not reached under /api/v1/changes")
	}
}

func TestRecoverMiddleware(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "internal" {
		t.Errorf("code = %q, want internal", body.Error.Code)
	}
}

func TestRouterMountsSkills(t *testing.T) {
	reached := false
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if r.URL.Path != "" && r.URL.Path != "/" {
			t.Errorf("stripped path = %q, want root", r.URL.Path)
		}
	})
	h := NewRouter(Deps{Skills: stub})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !reached {
		t.Error("skills handler not reached under /api/v1/skills")
	}
}

func TestRouterMountsAgentsAndRuns(t *testing.T) {
	agentsReached, runsReached := false, false
	agentsStub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		agentsReached = true
	})
	runsStub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		runsReached = true
	})
	h := NewRouter(Deps{Agents: agentsStub, Runs: runsStub})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !agentsReached {
		t.Error("agents handler not reached under /api/v1/agents")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/r1", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !runsReached {
		t.Error("runs handler not reached under /api/v1/runs")
	}
}
