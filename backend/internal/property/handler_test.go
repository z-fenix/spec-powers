package property

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
)

type handlerFixture struct {
	handler http.Handler
	tokens  *auth.TokenService
	svc     *Service
	props   *fakeProperties
	issues  *fakeIssueStore
}

func setupHandler(t *testing.T) *handlerFixture {
	t.Helper()
	issues := &fakeIssueStore{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", ProjectID: "p1", Title: "one"},
	}}
	props := newFakeProperties(issues)
	projects := &fakeProjects{
		existing: map[string]bool{"p1": true},
		members:  map[string]string{"p1|alice": "owner", "p1|bob": "member"},
	}
	svc := NewService(props, projects, issues)
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(svc, tokens)

	r := chi.NewRouter()
	r.Route("/{projectID}", func(r chi.Router) {
		r.Mount("/properties", h.DefinitionRoutes())
		r.Route("/issues/{issueID}", func(r chi.Router) {
			r.Mount("/properties", h.ValueRoutes())
		})
	})
	return &handlerFixture{handler: r, tokens: tokens, svc: svc, props: props, issues: issues}
}

func (f *handlerFixture) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := f.tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (f *handlerFixture) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
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
	f.handler.ServeHTTP(w, req)
	return w
}

func TestPropertyDefinitionRoutes(t *testing.T) {
	f := setupHandler(t)
	owner := f.token(t, "alice")
	member := f.token(t, "bob")

	t.Run("owner creates a definition", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/properties", owner, map[string]any{
			"name": "模块", "type": "select", "options": []string{"前端", "后端"},
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Property definitionDTO `json:"property"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Property.Name != "模块" || body.Property.Type != "select" || len(body.Property.Options) != 2 {
			t.Errorf("property = %+v", body.Property)
		}
	})

	t.Run("member cannot create", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/properties", member, map[string]any{"name": "x", "type": "text"})
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("list requires no special role", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/properties", member, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("malformed body is 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/p1/properties", bytes.NewReader([]byte("{oops")))
		req.Header.Set("Authorization", "Bearer "+owner)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestPropertyValueRoutes(t *testing.T) {
	f := setupHandler(t)
	member := f.token(t, "bob")

	f.do(t, http.MethodPost, "/p1/properties", f.token(t, "alice"), map[string]any{
		"name": "模块", "type": "select", "options": []string{"前端", "后端"},
	})
	var defID string
	{
		w := f.do(t, http.MethodGet, "/p1/properties", member, nil)
		var body struct {
			Properties []definitionDTO `json:"properties"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		defID = body.Properties[0].ID
	}

	t.Run("set and list a value", func(t *testing.T) {
		w := f.do(t, http.MethodPut, "/p1/issues/i1/properties/"+defID, member, map[string]any{"value": "后端"})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		w = f.do(t, http.MethodGet, "/p1/issues/i1/properties", member, nil)
		var body struct {
			Values []valueDTO `json:"values"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Values) != 1 || body.Values[0].Value != "后端" || body.Values[0].PropertyID != defID {
			t.Errorf("values = %+v", body.Values)
		}
	})

	t.Run("invalid select value is 400", func(t *testing.T) {
		w := f.do(t, http.MethodPut, "/p1/issues/i1/properties/"+defID, member, map[string]any{"value": "测试"})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("delete the value", func(t *testing.T) {
		w := f.do(t, http.MethodDelete, "/p1/issues/i1/properties/"+defID, member, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d", w.Code)
		}
		w = f.do(t, http.MethodGet, "/p1/issues/i1/properties", member, nil)
		var body struct {
			Values []valueDTO `json:"values"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Values) != 0 {
			t.Errorf("values = %+v, want empty", body.Values)
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/i1/properties", "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}
