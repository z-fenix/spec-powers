package property

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

type definitionDTO struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"project_id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Options   []string `json:"options"`
	Position  int      `json:"position"`
}

func toDefinitionDTO(d *domain.PropertyDefinition) definitionDTO {
	options := d.Options
	if options == nil {
		options = []string{}
	}
	return definitionDTO{
		ID: d.ID, ProjectID: d.ProjectID, Name: d.Name, Type: d.Type,
		Options: options, Position: d.Position,
	}
}

type valueDTO struct {
	IssueID    string `json:"issue_id"`
	PropertyID string `json:"property_id"`
	Value      string `json:"value"`
}

func toValueDTO(v *domain.IssuePropertyValue) valueDTO {
	return valueDTO{IssueID: v.IssueID, PropertyID: v.PropertyID, Value: v.Value}
}

// DefinitionRoutes serves property-definition CRUD; mount under
// /projects/{projectID}/properties.
func (h *Handler) DefinitionRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/", h.createDefinition)
	r.Get("/", h.listDefinitions)
	// Project-wide issue property values: the board uses them to filter
	// issues by property without per-issue reads.
	r.Get("/values", h.listProjectValues)
	r.Patch("/{propertyID}", h.updateDefinition)
	r.Delete("/{propertyID}", h.deleteDefinition)
	return r
}

// ValueRoutes serves issue property values; mount under
// /projects/{projectID}/issues/{issueID}/properties.
func (h *Handler) ValueRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.listValues)
	r.Put("/{propertyID}", h.setValue)
	r.Delete("/{propertyID}", h.deleteValue)
	return r
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httpapi.AppError); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

func (h *Handler) createDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Options []string `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	d, err := h.svc.CreateDefinition(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), DefinitionInput(req))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"property": toDefinitionDTO(d)})
}

func (h *Handler) listDefinitions(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListDefinitions(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]definitionDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toDefinitionDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"properties": dtos})
}

func (h *Handler) updateDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Options []string `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	d, err := h.svc.UpdateDefinition(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), chi.URLParam(r, "propertyID"), DefinitionInput(req))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"property": toDefinitionDTO(d)})
}

func (h *Handler) deleteDefinition(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteDefinition(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), chi.URLParam(r, "propertyID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listProjectValues(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListProjectValues(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]valueDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toValueDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"values": dtos})
}

func (h *Handler) listValues(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListIssueValues(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]valueDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toValueDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"values": dtos})
}

func (h *Handler) setValue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	v, err := h.svc.SetIssueValue(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"), chi.URLParam(r, "propertyID"), req.Value)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"value": toValueDTO(v)})
}

func (h *Handler) deleteValue(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteIssueValue(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"), chi.URLParam(r, "propertyID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
