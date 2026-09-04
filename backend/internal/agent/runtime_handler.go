package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/store"
)

// RuntimeHandlerDeps wires the runtime API: the endpoints a locally
// registered agent runtime calls to claim runs and drive their execution
// remotely (context reads, comments, status, logs, finish).
type RuntimeHandlerDeps struct {
	Agents     store.AgentStore
	Runs       store.RunStore
	Logs       store.RunLogStore
	Issues     store.IssueStore
	Comments   store.CommentStore
	Metadata   store.IssueMetadataStore
	Projects   store.ProjectStore
	Tokens     *auth.TokenService
	MentionHook func(ctx context.Context, issueID, authorID, content string) error
}

// RuntimeHandler serves the agent-runtime REST surface mounted at /runtime.
// Every route requires a valid bearer token whose subject is an existing
// agent (revoking a credential is deleting the agent).
type RuntimeHandler struct {
	deps RuntimeHandlerDeps
}

func NewRuntimeHandler(deps RuntimeHandlerDeps) *RuntimeHandler {
	return &RuntimeHandler{deps: deps}
}

func (h *RuntimeHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(h.requireAgent)
	r.Post("/claim", h.claim)
	r.Route("/runs/{runID}", func(r chi.Router) {
		r.Post("/log", h.appendLog)
		r.Post("/finish", h.finish)
	})
	// Issue-scoped endpoints authorize the calling agent by "has any run on
	// the issue" — the run queue is what grants an agent access to an issue.
	r.Route("/issues/{issueID}", func(r chi.Router) {
		r.Get("/", h.issueContext)
		r.Post("/comments", h.postComment)
		r.Post("/status", h.setStatus)
	})
	return r
}

type agentCtxKey int

const runtimeAgentKey agentCtxKey = 1

// requireAgent verifies the bearer token and that its subject is an
// existing agent row; the agent is injected into the request context.
func (h *RuntimeHandler) requireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpapi.Error(w, httpapi.ErrUnauthorized("missing bearer token"))
			return
		}
		userID, err := h.deps.Tokens.Verify(token)
		if err != nil {
			httpapi.Error(w, httpapi.ErrUnauthorized("invalid or expired token"))
			return
		}
		a, err := h.deps.Agents.GetAgent(r.Context(), userID)
		if err != nil {
			httpapi.Error(w, httpapi.ErrUnauthorized("not an agent identity"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), runtimeAgentKey, a)))
	})
}

func runtimeAgentFrom(ctx context.Context) *domain.Agent {
	a, _ := ctx.Value(runtimeAgentKey).(*domain.Agent)
	return a
}

type issueDTO struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ParentID    string `json:"parent_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	AssigneeID  string `json:"assignee_id,omitempty"`
}

type commentDTO struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	AuthorID string `json:"author_id"`
	Content  string `json:"content"`
}

type issueMetadataDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

type projectResourceDTO struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	Pointer string `json:"pointer"`
}

func (h *RuntimeHandler) claim(w http.ResponseWriter, r *http.Request) {
	a := runtimeAgentFrom(r.Context())
	run, err := h.deps.Runs.ClaimNextRunForAgent(r.Context(), a.ID)
	if err == store.ErrNotFound {
		httpapi.JSON(w, http.StatusOK, map[string]any{"run": nil})
		return
	}
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("claim run failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"run": toRunDTO(run)})
}

// ownedRun loads the run and enforces that it belongs to the calling agent.
func (h *RuntimeHandler) ownedRun(ctx context.Context, runID string) (*domain.Run, error) {
	a := runtimeAgentFrom(ctx)
	run, err := h.deps.Runs.GetRun(ctx, runID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("run not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get run failed")
	}
	if run.AgentID != a.ID {
		return nil, httpapi.ErrForbidden("run belongs to another agent")
	}
	return run, nil
}

// agentOnIssue authorizes the calling agent for an issue: the agent must
// hold a run on it (any status). Returns the issue on success.
func (h *RuntimeHandler) agentOnIssue(ctx context.Context, issueID string) (*domain.Issue, error) {
	iss, err := h.deps.Issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get issue failed")
	}
	a := runtimeAgentFrom(ctx)
	runs, err := h.deps.Runs.ListRuns(ctx, store.RunFilter{IssueID: issueID, AgentID: a.ID})
	if err != nil {
		return nil, httpapi.ErrInternal("list runs failed")
	}
	if len(runs) == 0 {
		return nil, httpapi.ErrForbidden("no run of this agent on the issue")
	}
	return iss, nil
}

func (h *RuntimeHandler) issueContext(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	iss, err := h.agentOnIssue(ctx, chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	comments, _ := h.deps.Comments.ListComments(ctx, iss.ID)
	metadata, _ := h.deps.Metadata.ListIssueMetadata(ctx, iss.ID)
	resources, _ := h.deps.Projects.ListProjectResources(ctx, iss.ProjectID)

	httpapi.JSON(w, http.StatusOK, map[string]any{
		"issue":     toIssueDTO(iss),
		"comments":  toCommentDTOs(comments),
		"metadata":  toMetadataDTOs(metadata),
		"resources": toResourceDTOs(resources),
	})
}

func toIssueDTO(i *domain.Issue) *issueDTO {
	return &issueDTO{
		ID: i.ID, ProjectID: i.ProjectID, ParentID: i.ParentID,
		Title: i.Title, Description: i.Description, Status: i.Status, AssigneeID: i.AssigneeID,
	}
}

func toCommentDTOs(list []domain.IssueComment) []commentDTO {
	out := make([]commentDTO, 0, len(list))
	for _, c := range list {
		out = append(out, commentDTO{ID: c.ID, ParentID: c.ParentID, AuthorID: c.AuthorID, Content: c.Content})
	}
	return out
}

func toMetadataDTOs(list []domain.IssueMetadata) []issueMetadataDTO {
	out := make([]issueMetadataDTO, 0, len(list))
	for _, m := range list {
		out = append(out, issueMetadataDTO{Key: m.Key, Value: m.Value, Type: m.Type})
	}
	return out
}

func toResourceDTOs(list []domain.ProjectResource) []projectResourceDTO {
	out := make([]projectResourceDTO, 0, len(list))
	for _, r := range list {
		out = append(out, projectResourceDTO{ID: r.ID, Type: r.Type, Label: r.Label, Pointer: r.Pointer})
	}
	return out
}

func (h *RuntimeHandler) postComment(w http.ResponseWriter, r *http.Request) {
	iss, err := h.agentOnIssue(r.Context(), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req struct {
		Content         string `json:"content"`
		ParentCommentID string `json:"parent_comment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	a := runtimeAgentFrom(r.Context())
	if strings.TrimSpace(req.Content) == "" {
		httpapi.Error(w, httpapi.ErrInvalid("content is required"))
		return
	}
	c, err := h.deps.Comments.CreateComment(r.Context(), &domain.IssueComment{
		IssueID:  iss.ID,
		ParentID: req.ParentCommentID,
		AuthorID: a.ID,
		Content:  req.Content,
	})
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("create comment failed"))
		return
	}
	if h.deps.MentionHook != nil {
		if err := h.deps.MentionHook(r.Context(), iss.ID, a.ID, req.Content); err != nil {
			httpapi.Error(w, httpapi.ErrInternal("mention trigger failed"))
			return
		}
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"comment_id": c.ID})
}

func (h *RuntimeHandler) setStatus(w http.ResponseWriter, r *http.Request) {
	iss, err := h.agentOnIssue(r.Context(), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	if _, err := issue.Transition(iss.Status, req.Status); err != nil {
		writeAppError(w, err)
		return
	}
	updated := *iss
	updated.Status = req.Status
	if _, err := h.deps.Issues.UpdateIssue(r.Context(), &updated); err != nil {
		httpapi.Error(w, httpapi.ErrInternal("update issue failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"status": req.Status})
}

func (h *RuntimeHandler) appendLog(w http.ResponseWriter, r *http.Request) {
	run, err := h.ownedRun(r.Context(), chi.URLParam(r, "runID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	l, err := h.deps.Logs.AppendRunLog(r.Context(), &domain.RunLog{
		RunID: run.ID, Kind: req.Kind, Content: req.Content,
	})
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("append run log failed"))
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"seq": l.Seq})
}

func (h *RuntimeHandler) finish(w http.ResponseWriter, r *http.Request) {
	run, err := h.ownedRun(r.Context(), chi.URLParam(r, "runID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	if req.Status != "done" && req.Status != "failed" {
		httpapi.Error(w, httpapi.ErrInvalid("status must be done or failed"))
		return
	}
	updated, err := h.deps.Runs.FinishRun(r.Context(), run.ID, req.Status, req.Error)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("finish run failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"run": toRunDTO(updated)})
}
