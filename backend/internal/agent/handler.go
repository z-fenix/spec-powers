package agent

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// RuntimeTokenTTL is how long a locally registered agent's runtime
// credential stays valid. Revocation is deleting the agent (deregister):
// runtime endpoints require an existing agent row.
const RuntimeTokenTTL = 365 * 24 * time.Hour

// Handler serves the agent and run REST endpoints. AgentRoutes mounts at
// /agents, RunRoutes at /runs.
type Handler struct {
	svc    *Service
	queue  *Queue
	runs   store.RunStore
	logs   store.RunLogStore
	issues store.IssueStore
	tokens *auth.TokenService
	// runtimeTokens issues the long-lived credentials returned by
	// POST /agents/register; same secret as tokens.
	runtimeTokens *auth.TokenService
}

func NewHandler(svc *Service, queue *Queue, runs store.RunStore, logs store.RunLogStore, issues store.IssueStore, tokens, runtimeTokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, queue: queue, runs: runs, logs: logs, issues: issues, tokens: tokens, runtimeTokens: runtimeTokens}
}

func (h *Handler) AgentRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/", h.createAgent)
	r.Post("/register", h.registerAgent)
	r.Get("/", h.listAgents)
	r.Route("/{agentID}", func(r chi.Router) {
		r.Get("/", h.getAgent)
		r.Patch("/", h.updateAgent)
		r.Delete("/", h.removeAgent)
	})
	return r
}

func (h *Handler) RunRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/", h.triggerRun)
	r.Get("/", h.listRuns)
	r.Get("/usage", h.usageTotals)
	r.Get("/{runID}", h.getRun)
	return r
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*httpapi.AppError); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

type agentDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
	Runtime     string   `json:"runtime"`
	CreatedBy   string   `json:"created_by"`
}

func toAgentDTO(a *domain.Agent) agentDTO {
	skills := a.Skills
	if skills == nil {
		skills = []string{}
	}
	runtime := a.Runtime
	if runtime == "" {
		runtime = "server"
	}
	return agentDTO{ID: a.ID, Name: a.Name, Description: a.Description, Skills: skills, Runtime: runtime, CreatedBy: a.CreatedBy}
}

func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Skills      []string `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	a, err := h.svc.CreateAgent(r.Context(), auth.UserIDFrom(r.Context()), CreateInput{
		Name:        req.Name,
		Description: req.Description,
		Skills:      req.Skills,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"agent": toAgentDTO(a)})
}

// registerAgent creates a local-runtime agent and issues its runtime
// credential (a long-lived bearer token for the agent's identity).
func (h *Handler) registerAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Skills      []string `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	a, err := h.svc.CreateAgent(r.Context(), auth.UserIDFrom(r.Context()), CreateInput{
		Name:        req.Name,
		Description: req.Description,
		Skills:      req.Skills,
		Runtime:     "local",
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	token, err := h.runtimeTokens.Issue(a.ID)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("issue runtime token failed"))
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"agent": toAgentDTO(a), "token": token})
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListAgents(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]agentDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toAgentDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"agents": dtos})
}

func (h *Handler) getAgent(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.GetAgent(r.Context(), chi.URLParam(r, "agentID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"agent": toAgentDTO(a)})
}

func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Skills      *[]string `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	in := UpdateInput{Name: req.Name, Description: req.Description}
	if req.Skills != nil {
		in.Skills = *req.Skills
	}
	a, err := h.svc.UpdateAgent(r.Context(), chi.URLParam(r, "agentID"), in)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"agent": toAgentDTO(a)})
}

func (h *Handler) removeAgent(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteAgent(r.Context(), chi.URLParam(r, "agentID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type runDTO struct {
	ID         string  `json:"id"`
	AgentID    string  `json:"agent_id"`
	IssueID    string  `json:"issue_id"`
	Trigger    string  `json:"trigger"`
	Status     string  `json:"status"`
	Error      string  `json:"error"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at,omitempty"`
	FinishedAt *string `json:"finished_at,omitempty"`
}

func toRunDTO(r *domain.Run) runDTO {
	format := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		s := t.UTC().Format(time.RFC3339)
		return &s
	}
	return runDTO{
		ID: r.ID, AgentID: r.AgentID, IssueID: r.IssueID, Trigger: r.Trigger,
		Status: r.Status, Error: r.Error, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		StartedAt: format(r.StartedAt), FinishedAt: format(r.FinishedAt),
	}
}

type runLogDTO struct {
	Seq     int    `json:"seq"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

func toRunLogDTOs(list []domain.RunLog) []runLogDTO {
	out := make([]runLogDTO, 0, len(list))
	for _, l := range list {
		out = append(out, runLogDTO{Seq: l.Seq, Kind: l.Kind, Content: l.Content})
	}
	return out
}

func (h *Handler) triggerRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IssueID string `json:"issue_id"`
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	if _, err := h.svc.GetAgent(r.Context(), req.AgentID); err != nil {
		writeAppError(w, err)
		return
	}
	if _, err := h.issues.GetIssue(r.Context(), req.IssueID); err != nil {
		if err == store.ErrNotFound {
			httpapi.Error(w, httpapi.ErrNotFound("issue not found"))
			return
		}
		httpapi.Error(w, httpapi.ErrInternal("get issue failed"))
		return
	}
	run, err := h.queue.Enqueue(r.Context(), req.AgentID, req.IssueID, "manual")
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"run": toRunDTO(run)})
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.RunFilter{
		IssueID: q.Get("issue_id"),
		AgentID: q.Get("agent_id"),
		Status:  q.Get("status"),
	}
	list, err := h.runs.ListRuns(r.Context(), filter)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("list runs failed"))
		return
	}
	dtos := make([]runDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toRunDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"runs": dtos})
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.runs.GetRun(r.Context(), chi.URLParam(r, "runID"))
	if err == store.ErrNotFound {
		httpapi.Error(w, httpapi.ErrNotFound("run not found"))
		return
	}
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("get run failed"))
		return
	}
	logs, err := h.logs.ListRunLogs(r.Context(), run.ID)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("list run logs failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"run": toRunDTO(run), "logs": toRunLogDTOs(logs)})
}

// ---- usage ----

type usageTotalsDTO struct {
	Calls            int   `json:"calls"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type issueUsageDTO struct {
	IssueID          string `json:"issue_id"`
	Title            string `json:"title"`
	Calls            int    `json:"calls"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

// usageTotals serves GET /runs/usage?issue_id=... or ?project_id=...:
// LLM token usage aggregated per issue (issue query) or per issue of one
// project (project query). Exactly one of the two params is required.
func (h *Handler) usageTotals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	issueID, projectID := q.Get("issue_id"), q.Get("project_id")
	if (issueID == "") == (projectID == "") {
		httpapi.Error(w, httpapi.ErrInvalid("exactly one of issue_id or project_id is required"))
		return
	}
	if issueID != "" {
		t, err := h.runs.IssueUsage(r.Context(), issueID)
		if err != nil {
			httpapi.Error(w, httpapi.ErrInternal("issue usage failed"))
			return
		}
		httpapi.JSON(w, http.StatusOK, map[string]any{
			"issue_id": issueID,
			"usage": usageTotalsDTO{
				Calls: t.Calls, PromptTokens: t.PromptTokens, CompletionTokens: t.CompletionTokens,
			},
		})
		return
	}
	list, err := h.runs.ProjectUsage(r.Context(), projectID)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("project usage failed"))
		return
	}
	dtos := make([]issueUsageDTO, 0, len(list))
	for _, u := range list {
		dtos = append(dtos, issueUsageDTO{
			IssueID: u.IssueID, Title: u.Title,
			Calls: u.Calls, PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"project_id": projectID, "usage": dtos})
}
