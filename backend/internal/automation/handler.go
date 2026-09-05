package automation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

// Handler serves the automation management API: webhook CRUD under
// WebhookRoutes, autopilot CRUD + manual trigger under AutopilotRoutes and
// the unauthenticated inbound endpoint under HookRoutes.
type Handler struct {
	svc    *Service
	tokens *auth.TokenService
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

type webhookDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Secret    string `json:"secret"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

func toWebhookDTO(w domain.Webhook) webhookDTO {
	return webhookDTO{
		ID: w.ID, Name: w.Name, Secret: w.Secret, Enabled: w.Enabled,
		CreatedAt: w.CreatedAt.UTC().Format(timeFormat),
	}
}

type autopilotDTO struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	TriggerType      string  `json:"trigger_type"`
	CronSpec         string  `json:"cron_spec"`
	WebhookID        string  `json:"webhook_id"`
	ActionType       string  `json:"action_type"`
	AgentID          string  `json:"agent_id"`
	ProjectID        string  `json:"project_id"`
	IssueID          string  `json:"issue_id"`
	IssueTitle       string  `json:"issue_title"`
	IssueDescription string  `json:"issue_description"`
	Enabled          bool    `json:"enabled"`
	LastFiredAt      *string `json:"last_fired_at"`
	NextRunAt        *string `json:"next_run_at"`
	CreatedAt        string  `json:"created_at"`
}

func toAutopilotDTO(a domain.Autopilot) autopilotDTO {
	dto := autopilotDTO{
		ID: a.ID, Name: a.Name, TriggerType: a.TriggerType, CronSpec: a.CronSpec,
		WebhookID: a.WebhookID, ActionType: a.ActionType,
		AgentID: a.AgentID, ProjectID: a.ProjectID, IssueID: a.IssueID,
		IssueTitle: a.IssueTitle, IssueDescription: a.IssueDescription,
		Enabled:   a.Enabled,
		CreatedAt: a.CreatedAt.UTC().Format(timeFormat),
	}
	if a.LastFiredAt != nil {
		fired := a.LastFiredAt.UTC().Format(timeFormat)
		dto.LastFiredAt = &fired
	}
	if a.NextRunAt != nil {
		next := a.NextRunAt.UTC().Format(timeFormat)
		dto.NextRunAt = &next
	}
	return dto
}

// WebhookRoutes returns the authenticated webhook management API.
func (h *Handler) WebhookRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.listWebhooks)
	r.Post("/", h.createWebhook)
	r.Get("/{webhookID}", h.getWebhook)
	r.Patch("/{webhookID}", h.updateWebhook)
	r.Delete("/{webhookID}", h.deleteWebhook)
	return r
}

// AutopilotRoutes returns the authenticated autopilot management API.
func (h *Handler) AutopilotRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.listAutopilots)
	r.Post("/", h.createAutopilot)
	r.Get("/{autopilotID}", h.getAutopilot)
	r.Put("/{autopilotID}", h.updateAutopilot)
	r.Delete("/{autopilotID}", h.deleteAutopilot)
	r.Post("/{autopilotID}/trigger", h.triggerAutopilot)
	return r
}

func decodeBody[T any](w http.ResponseWriter, r *http.Request, into *T) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("invalid JSON body"))
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, err error) {
	var appErr *httpapi.AppError
	if errors.As(err, &appErr) {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("automation request failed"))
}

func (h *Handler) listWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListWebhooks(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	dtos := make([]webhookDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toWebhookDTO(list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"webhooks": dtos})
}

func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	hook, err := h.svc.CreateWebhook(r.Context(), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"webhook": toWebhookDTO(*hook)})
}

func (h *Handler) getWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := h.svc.GetWebhook(r.Context(), chi.URLParam(r, "webhookID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"webhook": toWebhookDTO(*hook)})
}

func (h *Handler) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	hook, err := h.svc.UpdateWebhook(r.Context(), chi.URLParam(r, "webhookID"), body.Name, body.Enabled)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"webhook": toWebhookDTO(*hook)})
}

func (h *Handler) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteWebhook(r.Context(), chi.URLParam(r, "webhookID"))
	if isNotFound(err) {
		httpapi.Error(w, httpapi.ErrNotFound("webhook not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) listAutopilots(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListAutopilots(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	dtos := make([]autopilotDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toAutopilotDTO(list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"autopilots": dtos})
}

func (h *Handler) createAutopilot(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeAutopilotInput(w, r)
	if !ok {
		return
	}
	a, err := h.svc.CreateAutopilot(r.Context(), auth.UserIDFrom(r.Context()), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"autopilot": toAutopilotDTO(*a)})
}

func (h *Handler) getAutopilot(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.GetAutopilot(r.Context(), chi.URLParam(r, "autopilotID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"autopilot": toAutopilotDTO(*a)})
}

func (h *Handler) updateAutopilot(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeAutopilotInput(w, r)
	if !ok {
		return
	}
	a, err := h.svc.UpdateAutopilot(r.Context(), chi.URLParam(r, "autopilotID"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"autopilot": toAutopilotDTO(*a)})
}

func (h *Handler) deleteAutopilot(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteAutopilot(r.Context(), chi.URLParam(r, "autopilotID"))
	if isNotFound(err) {
		httpapi.Error(w, httpapi.ErrNotFound("autopilot not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) triggerAutopilot(w http.ResponseWriter, r *http.Request) {
	var payload EventPayload
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload) // empty or non-JSON body = empty payload
	}
	if err := h.svc.TriggerAutopilot(r.Context(), chi.URLParam(r, "autopilotID"), payload); err != nil {
		writeErr(w, err)
		return
	}
	httpapi.JSON(w, http.StatusAccepted, map[string]any{"triggered": true})
}

func decodeAutopilotInput(w http.ResponseWriter, r *http.Request) (AutopilotInput, bool) {
	var body struct {
		Name             string `json:"name"`
		TriggerType      string `json:"trigger_type"`
		CronSpec         string `json:"cron_spec"`
		WebhookID        string `json:"webhook_id"`
		ActionType       string `json:"action_type"`
		AgentID          string `json:"agent_id"`
		ProjectID        string `json:"project_id"`
		IssueID          string `json:"issue_id"`
		IssueTitle       string `json:"issue_title"`
		IssueDescription string `json:"issue_description"`
		Enabled          *bool  `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return AutopilotInput{}, false
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	return AutopilotInput{
		Name:             body.Name,
		TriggerType:      body.TriggerType,
		CronSpec:         body.CronSpec,
		WebhookID:        body.WebhookID,
		ActionType:       body.ActionType,
		AgentID:          body.AgentID,
		ProjectID:        body.ProjectID,
		IssueID:          body.IssueID,
		IssueTitle:       body.IssueTitle,
		IssueDescription: body.IssueDescription,
		Enabled:          enabled,
	}, true
}
