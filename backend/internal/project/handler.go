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
	// issues is the issue-domain subrouter mounted under
	// /{projectID}/issues; nil in tests that don't exercise issues.
	issues http.Handler
	// pullRequests is the PR subrouter mounted under
	// /{projectID}/pullrequests; nil in tests that don't exercise PRs.
	pullRequests http.Handler
	// properties is the property-definition subrouter mounted under
	// /{projectID}/properties; nil in tests that don't exercise properties.
	properties http.Handler
}

func NewHandler(svc *Service, tokens *auth.TokenService, issues http.Handler) *Handler {
	return &Handler{svc: svc, tokens: tokens, issues: issues}
}

// WithPullRequests attaches the pull-request subrouter served under
// /{projectID}/pullrequests.
func (h *Handler) WithPullRequests(p http.Handler) *Handler {
	h.pullRequests = p
	return h
}

// WithProperties attaches the property-definition subrouter served under
// /{projectID}/properties.
func (h *Handler) WithProperties(p http.Handler) *Handler {
	h.properties = p
	return h
}

type projectDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Key         string `json:"key"`
	Archived    bool   `json:"archived"`
	CreatedBy   string `json:"created_by"`
}

func toProjectDTO(p *domain.Project) projectDTO {
	return projectDTO{
		ID: p.ID, WorkspaceID: p.WorkspaceID, Name: p.Name,
		Description: p.Description, Key: p.Key, Archived: p.Archived, CreatedBy: p.CreatedBy,
	}
}

type memberDTO struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type resourceDTO struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	Pointer string `json:"pointer"`
}

func toResourceDTO(r *domain.ProjectResource) resourceDTO {
	return resourceDTO{ID: r.ID, Type: r.Type, Label: r.Label, Pointer: r.Pointer}
}

// requireRole is the project-level permission middleware: every route under
// /{projectID} requires at least the given role (owner passes member too).
func (h *Handler) requireRole(minRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := chi.URLParam(r, "projectID")
			err := h.svc.RequireProjectRole(r.Context(), auth.UserIDFrom(r.Context()), projectID, minRole)
			if err != nil {
				writeAppError(w, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(h.tokens))
		r.Post("/", h.create)
		r.Get("/", h.list)
		r.Route("/{projectID}", func(r chi.Router) {
			r.Use(h.requireRole("member"))
			r.Get("/", h.get)
			r.Get("/resources", h.listResources)
			r.Get("/context", h.getContext)
			if h.issues != nil {
				r.Mount("/issues", h.issues)
			}
			if h.pullRequests != nil {
				r.Mount("/pullrequests", h.pullRequests)
			}
			if h.properties != nil {
				r.Mount("/properties", h.properties)
			}
			r.Group(func(r chi.Router) {
				r.Use(h.requireRole("owner"))
				r.Patch("/", h.update)
				r.Post("/archive", h.archive)
				r.Post("/members", h.addMember)
				r.Post("/resources", h.addResource)
				r.Delete("/resources/{resourceID}", h.removeResource)
				r.Put("/context", h.setContext)
			})
		})
	})
	return r
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Key         string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	p, err := h.svc.CreateProject(r.Context(), auth.UserIDFrom(r.Context()), req.Name, req.Description, req.Key)
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

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetProject(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"project": toProjectDTO(p)})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	p, err := h.svc.UpdateProject(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), req.Name, req.Description)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"project": toProjectDTO(p)})
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Archived bool `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	p, err := h.svc.SetArchived(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), req.Archived)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"project": toProjectDTO(p)})
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

func (h *Handler) listResources(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListResources(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]resourceDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toResourceDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"resources": dtos})
}

func (h *Handler) addResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string `json:"type"`
		Label   string `json:"label"`
		Pointer string `json:"pointer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	res, err := h.svc.AddResource(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), req.Type, req.Label, req.Pointer)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"resource": toResourceDTO(res)})
}

func (h *Handler) removeResource(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RemoveResource(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), chi.URLParam(r, "resourceID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getContext(w http.ResponseWriter, r *http.Request) {
	pc, err := h.svc.GetContext(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"context": map[string]string{"content": pc.Content}})
}

func (h *Handler) setContext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	pc, err := h.svc.SetContext(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), req.Content)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"context": map[string]string{"content": pc.Content}})
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httpapi.AppError); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}
