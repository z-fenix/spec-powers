package project

import (
	"encoding/json"
	"net/http"

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

type projectDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	CreatedBy   string `json:"created_by"`
}

func toProjectDTO(p *domain.Project) projectDTO {
	return projectDTO{ID: p.ID, WorkspaceID: p.WorkspaceID, Name: p.Name, CreatedBy: p.CreatedBy}
}

type memberDTO struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(h.tokens))
		r.Post("/", h.create)
		r.Get("/", h.list)
		r.Post("/{projectID}/members", h.addMember)
	})
	return r
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	p, err := h.svc.CreateProject(r.Context(), auth.UserIDFrom(r.Context()), req.Name)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"project": toProjectDTO(p)})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListProjects(r.Context(), auth.UserIDFrom(r.Context()))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]projectDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toProjectDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"projects": dtos})
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	pm, err := h.svc.AddMember(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), req.Email, req.Role)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"member": memberDTO{UserID: pm.UserID, Role: pm.Role}})
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httpapi.AppError); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}
