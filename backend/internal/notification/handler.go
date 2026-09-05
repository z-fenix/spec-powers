package notification

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

const timeFormat = "2006-01-02T15:04:05Z07:00"

type Handler struct {
	svc    *Service
	tokens *auth.TokenService
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

type notificationDTO struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	IssueID   string  `json:"issue_id"`
	ProjectID string  `json:"project_id"`
	Read      bool    `json:"read"`
	ReadAt    *string `json:"read_at"`
	CreatedAt string  `json:"created_at"`
}

func toDTO(n domain.Notification) notificationDTO {
	dto := notificationDTO{
		ID: n.ID, UserID: n.UserID, Kind: n.Kind, Title: n.Title, Body: n.Body,
		IssueID: n.IssueID, ProjectID: n.ProjectID,
		CreatedAt: n.CreatedAt.UTC().Format(timeFormat),
	}
	if n.ReadAt != nil {
		read := n.ReadAt.UTC().Format(timeFormat)
		dto.Read = true
		dto.ReadAt = &read
	}
	return dto
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.list)
	r.Post("/read-all", h.markAllRead)
	r.Post("/{notificationID}/read", h.markRead)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFrom(r.Context())
	list, err := h.svc.List(r.Context(), userID,
		r.URL.Query().Get("unread") == "true",
		r.URL.Query().Get("kind"))
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("list notifications failed"))
		return
	}
	unread, err := h.svc.CountUnread(r.Context(), userID)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("count unread failed"))
		return
	}
	dtos := make([]notificationDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toDTO(list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"notifications": dtos, "unread": unread})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.MarkRead(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "notificationID"))
	if err == store.ErrNotFound {
		httpapi.Error(w, httpapi.ErrNotFound("notification not found"))
		return
	}
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("mark read failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"notification": toDTO(*n)})
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	marked, err := h.svc.MarkAllRead(r.Context(), auth.UserIDFrom(r.Context()))
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("mark all read failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"marked": marked})
}
