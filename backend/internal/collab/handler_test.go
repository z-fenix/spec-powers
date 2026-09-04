package collab

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
)

type handlerFixture struct {
	handler http.Handler
	tokens  *auth.TokenService
	svc     *Service
}

func setupHandler(t *testing.T) *handlerFixture {
	t.Helper()
	f := newFixture(t)
	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(f.svc, tokens)

	// mirror the production mount: collab lives under
	// /projects/{projectID}/issues/{issueID}
	r := chi.NewRouter()
	r.Route("/{projectID}/issues/{issueID}", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	return &handlerFixture{handler: r, tokens: tokens, svc: f.svc}
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
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func (f *handlerFixture) upload(t *testing.T, path, token, fileName, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": fileName}))
	hdr.Set("Content-Type", "text/plain")
	fw, _ := mw.CreatePart(hdr)
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func TestCommentHandler(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	t.Run("create returns 201 with comment", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues/i1/comments", tok, map[string]any{"content": "first"})
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Comment commentDTO `json:"comment"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Comment.Content != "first" || body.Comment.AuthorID != "alice" || body.Comment.ID == "" {
			t.Errorf("comment = %+v", body.Comment)
		}
	})

	t.Run("reply references its thread root", func(t *testing.T) {
		root := f.do(t, http.MethodPost, "/p1/issues/i1/comments", tok, map[string]any{"content": "root"})
		var rootBody struct {
			Comment commentDTO `json:"comment"`
		}
		_ = json.Unmarshal(root.Body.Bytes(), &rootBody)

		w := f.do(t, http.MethodPost, "/p1/issues/i1/comments", tok, map[string]any{
			"content": "re", "parent_id": rootBody.Comment.ID,
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Comment commentDTO `json:"comment"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Comment.ParentID != rootBody.Comment.ID {
			t.Errorf("parent = %q, want %q", body.Comment.ParentID, rootBody.Comment.ID)
		}
	})

	t.Run("blank content is 400", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues/i1/comments", tok, map[string]any{"content": "  "})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("list returns comments", func(t *testing.T) {
		f.do(t, http.MethodPost, "/p1/issues/i1/comments", tok, map[string]any{"content": "listed"})
		w := f.do(t, http.MethodGet, "/p1/issues/i1/comments", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var body struct {
			Comments []commentDTO `json:"comments"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Comments) == 0 {
			t.Error("no comments returned")
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		w := f.do(t, http.MethodPost, "/p1/issues/missing/comments", tok, map[string]any{"content": "x"})
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("requires auth", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/i1/comments", "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestAttachmentHandler(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	created := ""
	t.Run("upload returns 201 with attachment", func(t *testing.T) {
		w := f.upload(t, "/p1/issues/i1/attachments", tok, "notes.txt", "hello")
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Attachment attachmentDTO `json:"attachment"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Attachment.FileName != "notes.txt" || body.Attachment.SizeBytes != 5 || body.Attachment.ID == "" {
			t.Errorf("attachment = %+v", body.Attachment)
		}
		created = body.Attachment.ID
	})

	t.Run("download returns the stored bytes", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/i1/attachments/"+created, tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		if got := w.Body.String(); got != "hello" {
			t.Errorf("body = %q", got)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
			t.Errorf("content-type = %q", ct)
		}
		if cd := w.Header().Get("Content-Disposition"); cd == "" {
			t.Error("missing content-disposition")
		}
	})

	t.Run("download unknown attachment is 404", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/i1/attachments/ghost", tok, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("upload without file part is 400", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/p1/issues/i1/attachments", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("list returns attachments", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/i1/attachments", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var body struct {
			Attachments []attachmentDTO `json:"attachments"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Attachments) != 1 {
			t.Errorf("attachments = %+v", body.Attachments)
		}
	})
}

func TestMetadataHandler(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	t.Run("put creates then overwrites an entry", func(t *testing.T) {
		w := f.do(t, http.MethodPut, "/p1/issues/i1/metadata/env", tok, map[string]any{"value": "staging"})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Entry metadataDTO `json:"entry"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Entry.Key != "env" || body.Entry.Value != "staging" || body.Entry.Type != "string" {
			t.Errorf("entry = %+v", body.Entry)
		}

		w = f.do(t, http.MethodPut, "/p1/issues/i1/metadata/env", tok, map[string]any{"value": "prod"})
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Entry.Value != "prod" {
			t.Errorf("entry = %+v", body.Entry)
		}
	})

	t.Run("put with typed value validates", func(t *testing.T) {
		w := f.do(t, http.MethodPut, "/p1/issues/i1/metadata/est", tok, map[string]any{"value": "3.5", "type": "number"})
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		w = f.do(t, http.MethodPut, "/p1/issues/i1/metadata/bad", tok, map[string]any{"value": "abc", "type": "number"})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("list returns entries", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/i1/metadata", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var body struct {
			Metadata []metadataDTO `json:"metadata"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Metadata) != 2 {
			t.Errorf("metadata = %+v", body.Metadata)
		}
	})

	t.Run("delete removes then 404s", func(t *testing.T) {
		w := f.do(t, http.MethodDelete, "/p1/issues/i1/metadata/env", tok, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d", w.Code)
		}
		w = f.do(t, http.MethodDelete, "/p1/issues/i1/metadata/env", tok, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})
}
