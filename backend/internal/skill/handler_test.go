package skill

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
)

func TestSkillHandlerRoutes(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(reg, tokens)
	r := chi.NewRouter()
	r.Route("/skills", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	do := func(method, path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	tok, err := tokens.Issue("u1")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	t.Run("requires auth", func(t *testing.T) {
		if w := do(http.MethodGet, "/skills", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("lists skills in flow order", func(t *testing.T) {
		w := do(http.MethodGet, "/skills", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Skills []Skill `json:"skills"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Skills) != 3 {
			t.Fatalf("skills = %d, want 3", len(body.Skills))
		}
		want := []string{KeyBrainstorm, KeyWritePlan, KeySubagentDrivenDevelopment}
		for i, key := range want {
			if body.Skills[i].Key != key || body.Skills[i].Instructions == "" {
				t.Errorf("skills[%d] = %+v, want key %q with instructions", i, body.Skills[i], key)
			}
		}
	})

	t.Run("returns one skill by key", func(t *testing.T) {
		w := do(http.MethodGet, "/skills/"+KeyWritePlan, tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Skill Skill `json:"skill"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Skill.Key != KeyWritePlan || body.Skill.Name == "" {
			t.Errorf("skill = %+v", body.Skill)
		}
	})

	t.Run("unknown key is not found", func(t *testing.T) {
		if w := do(http.MethodGet, "/skills/nope", tok); w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}
