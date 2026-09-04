package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/collab"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// minimal working comment/attachment/metadata stubs so the collab handler
// can serve one comment during the wiring check.
type stubComments struct{}

func (stubComments) CreateComment(_ context.Context, c *domain.IssueComment) (*domain.IssueComment, error) {
	out := *c
	out.ID = "C-wired"
	return &out, nil
}

func (stubComments) GetComment(_ context.Context, _ string) (*domain.IssueComment, error) {
	return nil, store.ErrNotFound
}

func (stubComments) ListComments(_ context.Context, _ string) ([]domain.IssueComment, error) {
	return nil, nil
}

type stubAttachments struct {
	store.AttachmentStore
}

type stubMetadata struct {
	store.IssueMetadataStore
}

// TestIssueHandlerMountsCollab verifies the production wiring: the collab
// subrouter is reachable under /{projectID}/issues/{issueID}/... without
// breaking the issue routes on the same path.
func TestIssueHandlerMountsCollab(t *testing.T) {
	svc, _, _, _ := newService()
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	collabSvc := collab.NewService(svc.issues, svc.projects, stubComments{}, stubAttachments{}, stubMetadata{}, t.TempDir())
	collabHandler := collab.NewHandler(collabSvc, tokens).Routes()

	h := NewHandler(svc, tokens).WithCollab(collabHandler)
	r := chi.NewRouter()
	r.Route("/{projectID}", func(r chi.Router) {
		r.Mount("/issues", h.Routes())
	})

	tok, err := tokens.Issue("alice")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	created := ""
	t.Run("issue route still serves", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/p1/issues", bytes.NewReader([]byte(`{"title":"wired"}`)))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		created = body.Issue.ID
	})

	t.Run("collab route serves under the issue path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/p1/issues/"+created+"/comments", bytes.NewReader([]byte(`{"content":"hi"}`)))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Comment struct {
				ID      string `json:"id"`
				IssueID string `json:"issue_id"`
			} `json:"comment"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Comment.IssueID != created {
			t.Errorf("comment issue = %q, want %q", body.Comment.IssueID, created)
		}
	})
}
