package collab

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

type Handler struct {
	svc    *Service
	tokens *auth.TokenService
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

type commentDTO struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	ParentID  string `json:"parent_id"`
	AuthorID  string `json:"author_id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func toCommentDTO(c *domain.IssueComment) commentDTO {
	return commentDTO{
		ID: c.ID, IssueID: c.IssueID, ParentID: c.ParentID,
		AuthorID: c.AuthorID, Content: c.Content,
		CreatedAt: c.CreatedAt.UTC().Format(timeFormat),
	}
}

type attachmentDTO struct {
	ID          string `json:"id"`
	IssueID     string `json:"issue_id"`
	CommentID   string `json:"comment_id"`
	FileName    string `json:"file_name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	UploadedBy  string `json:"uploaded_by"`
	CreatedAt   string `json:"created_at"`
}

func toAttachmentDTO(a *domain.IssueAttachment) attachmentDTO {
	return attachmentDTO{
		ID: a.ID, IssueID: a.IssueID, CommentID: a.CommentID,
		FileName: a.FileName, SizeBytes: a.SizeBytes, ContentType: a.ContentType,
		UploadedBy: a.UploadedBy, CreatedAt: a.CreatedAt.UTC().Format(timeFormat),
	}
}

type metadataDTO struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updated_at"`
}

func toMetadataDTO(m *domain.IssueMetadata) metadataDTO {
	return metadataDTO{
		Key: m.Key, Value: m.Value, Type: m.Type,
		UpdatedAt: m.UpdatedAt.UTC().Format(timeFormat),
	}
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/comments", h.addComment)
	r.Get("/comments", h.listComments)
	r.Post("/attachments", h.uploadAttachment)
	r.Get("/attachments", h.listAttachments)
	r.Get("/attachments/{attachmentID}", h.downloadAttachment)
	r.Get("/metadata", h.listMetadata)
	r.Put("/metadata/{key}", h.setMetadata)
	r.Delete("/metadata/{key}", h.deleteMetadata)
	return r
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httpapi.AppError); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

func (h *Handler) addComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content  string `json:"content"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	c, err := h.svc.AddComment(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"), req.ParentID, req.Content)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"comment": toCommentDTO(c)})
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListComments(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]commentDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toCommentDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"comments": dtos})
}

func (h *Handler) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("multipart form required"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("file part is required"))
		return
	}
	defer file.Close()
	commentID := r.FormValue("comment_id")
	a, err := h.svc.AddAttachment(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"),
		commentID, path.Base(header.Filename), header.Header.Get("Content-Type"), file)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"attachment": toAttachmentDTO(a)})
}

func (h *Handler) listAttachments(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListAttachments(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]attachmentDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toAttachmentDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"attachments": dtos})
}

func (h *Handler) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	a, content, err := h.svc.GetAttachmentContent(r.Context(), auth.UserIDFrom(r.Context()),
		chi.URLParam(r, "issueID"), chi.URLParam(r, "attachmentID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	defer content.Close()
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": a.FileName}))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content)
}

func (h *Handler) listMetadata(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListMetadata(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]metadataDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toMetadataDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"metadata": dtos})
}

func (h *Handler) setMetadata(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
		Type  string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	m, err := h.svc.SetMetadata(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"),
		chi.URLParam(r, "key"), req.Value, req.Type)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"entry": toMetadataDTO(m)})
}

func (h *Handler) deleteMetadata(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteMetadata(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"), chi.URLParam(r, "key"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
