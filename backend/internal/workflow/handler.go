package workflow

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

const timeFormat = time.RFC3339

type Handler struct {
	svc    *Service
	tokens *auth.TokenService
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

type changeDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	IssueID   string `json:"issue_id"`
	Phase     string `json:"phase"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toChangeDTO(c *domain.Change) changeDTO {
	return changeDTO{
		ID: c.ID, ProjectID: c.ProjectID, IssueID: c.IssueID,
		Phase: c.Phase, Status: c.Status, CreatedBy: c.CreatedBy,
		CreatedAt: c.CreatedAt.UTC().Format(timeFormat),
		UpdatedAt: c.UpdatedAt.UTC().Format(timeFormat),
	}
}

type artifactDTO struct {
	ID        string `json:"id"`
	ChangeID  string `json:"change_id"`
	Kind      string `json:"kind"`
	Version   int    `json:"version"`
	Content   string `json:"content"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

func toArtifactDTO(a *domain.Artifact) artifactDTO {
	return artifactDTO{
		ID: a.ID, ChangeID: a.ChangeID, Kind: a.Kind, Version: a.Version,
		Content: a.Content, CreatedBy: a.CreatedBy,
		CreatedAt: a.CreatedAt.UTC().Format(timeFormat),
	}
}

type taskDTO struct {
	ID         string `json:"id"`
	ChangeID   string `json:"change_id"`
	ArtifactID string `json:"artifact_id"`
	IssueID    string `json:"issue_id"`
	Title      string `json:"title"`
	Stage      int    `json:"stage"`
	Position   int    `json:"position"`
}

func toTaskDTO(m *domain.TaskMapping) taskDTO {
	return taskDTO{
		ID: m.ID, ChangeID: m.ChangeID, ArtifactID: m.ArtifactID,
		IssueID: m.IssueID, Title: m.Title, Stage: m.Stage, Position: m.Position,
	}
}

type handoffDTO struct {
	ID        string `json:"id"`
	ChangeID  string `json:"change_id"`
	FromPhase string `json:"from_phase"`
	ToPhase   string `json:"to_phase"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

func toHandoffDTO(h *domain.ChangeHandoff) handoffDTO {
	return handoffDTO{
		ID: h.ID, ChangeID: h.ChangeID, FromPhase: h.FromPhase, ToPhase: h.ToPhase,
		CreatedBy: h.CreatedBy, CreatedAt: h.CreatedAt.UTC().Format(timeFormat),
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.byIssue)
	r.Post("/", h.startSplit)
	r.Route("/{changeID}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Get("/artifacts", h.listArtifacts)
		r.Get("/artifacts/{kind}", h.getArtifact)
		r.Post("/artifacts/{kind}", h.writeArtifact)
		r.Get("/tasks", h.listTasks)
		r.Get("/guard", h.guardStatus)
		r.Get("/skills/next", h.nextSkill)
		r.Post("/guard", h.advancePhase)
		r.Post("/verify", h.submitVerify)
		r.Post("/archive", h.archive)
	})
	return r
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httpapi.AppError); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

type startSplitRequest struct {
	IssueID string `json:"issue_id"`
	// Manual starts a bare proposal-phase change without the AI splitter,
	// for the agent-driven skill flow.
	Manual bool `json:"manual"`
}

// startSplit opens a change for an issue: by default it runs the AI classic
// split and returns the change plus the generated task mappings; with
// manual=true it creates a bare change for the skill flow to fill in.
func (h *Handler) startSplit(w http.ResponseWriter, r *http.Request) {
	var req startSplitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("body must be JSON with issue_id"))
		return
	}
	if req.IssueID == "" {
		httpapi.Error(w, httpapi.ErrInvalid("issue_id is required"))
		return
	}
	change, tasks, err := h.svc.StartChange(r.Context(), auth.UserIDFrom(r.Context()), req.IssueID, req.Manual)
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]taskDTO, 0, len(tasks))
	for i := range tasks {
		dtos = append(dtos, toTaskDTO(&tasks[i]))
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"change": toChangeDTO(change), "tasks": dtos})
}

func (h *Handler) byIssue(w http.ResponseWriter, r *http.Request) {
	issueID := r.URL.Query().Get("issue_id")
	if issueID == "" {
		httpapi.Error(w, httpapi.ErrInvalid("issue_id query parameter is required"))
		return
	}
	c, err := h.svc.GetChangeByIssue(r.Context(), auth.UserIDFrom(r.Context()), issueID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"change": toChangeDTO(c)})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetChange(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"change": toChangeDTO(c)})
}

func (h *Handler) listArtifacts(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListArtifacts(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]artifactDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toArtifactDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"artifacts": dtos})
}

func (h *Handler) getArtifact(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	version := 0
	if v := r.URL.Query().Get("version"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			httpapi.Error(w, httpapi.ErrInvalid("version must be an integer"))
			return
		}
		version = parsed
	}
	a, err := h.svc.GetArtifact(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"), kind, version)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"artifact": toArtifactDTO(a)})
}

type writeArtifactRequest struct {
	Content string `json:"content"`
}

// writeArtifact stores a new version of one artifact kind; the manual path
// used by the agent-driven skill flow.
func (h *Handler) writeArtifact(w http.ResponseWriter, r *http.Request) {
	var req writeArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("body must be JSON with content"))
		return
	}
	a, err := h.svc.WriteArtifact(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"), chi.URLParam(r, "kind"), req.Content)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"artifact": toArtifactDTO(a)})
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListTaskMappings(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]taskDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toTaskDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"tasks": dtos})
}

// guardStatus returns the change's gate evaluation without mutating state.
func (h *Handler) guardStatus(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.GuardStatus(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"guard": report})
}

// nextSkill resolves the skill the agent should load next for the change.
func (h *Handler) nextSkill(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.NextSkill(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"skill": s})
}

// advancePhase runs the guard-checked transition to the next classic phase.
func (h *Handler) advancePhase(w http.ResponseWriter, r *http.Request) {
	change, handoff, err := h.svc.AdvancePhase(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"change": toChangeDTO(change), "handoff": toHandoffDTO(handoff)})
}

type submitVerifyRequest struct {
	Content string `json:"content"`
}

// submitVerify stores a verify report; only result: pass releases the
// archive gate.
func (h *Handler) submitVerify(w http.ResponseWriter, r *http.Request) {
	var req submitVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("body must be JSON with content"))
		return
	}
	if req.Content == "" {
		httpapi.Error(w, httpapi.ErrInvalid("content is required"))
		return
	}
	a, passed, err := h.svc.SubmitVerifyReport(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"), req.Content)
	if err != nil {
		writeAppError(w, err)
		return
	}
	result := "fail"
	if passed {
		result = "pass"
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{
		"artifact": toArtifactDTO(a), "result": result, "passed": passed,
	})
}

// archive closes out the change after all gates pass.
func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Archive(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "changeID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"change": toChangeDTO(c)})
}
