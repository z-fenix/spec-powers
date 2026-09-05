package pr

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

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

type pullRequestDTO struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"project_id"`
	Repo       string   `json:"repo"`
	Number     int64    `json:"number"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	HeadBranch string   `json:"head_branch"`
	State      string   `json:"state"`
	MergedAt   string   `json:"merged_at"`
	IssueKeys  []string `json:"issue_keys"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

func toPullRequestDTO(pr *domain.PullRequest, issueKeys []string) pullRequestDTO {
	if issueKeys == nil {
		issueKeys = []string{}
	}
	merged := ""
	if pr.MergedAt != nil {
		merged = pr.MergedAt.Format(time.RFC3339)
	}
	return pullRequestDTO{
		ID: pr.ID, ProjectID: pr.ProjectID, Repo: pr.Repo, Number: pr.Number,
		Title: pr.Title, Body: pr.Body, HeadBranch: pr.HeadBranch, State: pr.State,
		MergedAt: merged, IssueKeys: issueKeys,
		CreatedAt: pr.CreatedAt.Format(time.RFC3339), UpdatedAt: pr.UpdatedAt.Format(time.RFC3339),
	}
}

// Routes serves the project-level PR endpoints; it is mounted under
// /{projectID}/pullrequests and reads the projectID URL param.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/", h.upsert)
	r.Get("/{prID}", h.get)
	r.Patch("/{prID}", h.updateState)
	return r
}

// IssueRoutes serves the issue-side listing; it is mounted under
// /{issueID}/pullrequests and reads the issueID URL param.
func (h *Handler) IssueRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.listForIssue)
	return r
}

func writeAppError(w http.ResponseWriter, err error) {
	var appErr *httpapi.AppError
	if errors.As(err, &appErr) {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo       string `json:"repo"`
		Number     int64  `json:"number"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		HeadBranch string `json:"head_branch"`
		State      string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	pr, linked, err := h.svc.UpsertPullRequest(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), UpsertInput{
		Repo:       req.Repo,
		Number:     req.Number,
		Title:      req.Title,
		Body:       req.Body,
		HeadBranch: req.HeadBranch,
		State:      req.State,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	dto := toPullRequestDTO(pr, linked)
	httpapi.JSON(w, http.StatusCreated, map[string]any{"pull_request": dto})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	pr, keys, err := h.svc.GetPullRequest(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "prID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"pull_request": toPullRequestDTO(pr, keys)})
}

func (h *Handler) updateState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	prID := chi.URLParam(r, "prID")
	pr, err := h.svc.UpdatePullRequestState(r.Context(), auth.UserIDFrom(r.Context()), prID, req.State)
	if err != nil {
		writeAppError(w, err)
		return
	}
	keys, err := h.svc.prs.ListLinkedIssues(r.Context(), prID)
	if err != nil {
		writeAppError(w, httpapi.ErrInternal("list linked issues failed"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"pull_request": toPullRequestDTO(pr, keys)})
}

func (h *Handler) listForIssue(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueID")
	list, err := h.svc.ListIssuePullRequests(r.Context(), auth.UserIDFrom(r.Context()), issueID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]pullRequestDTO, 0, len(list))
	for i := range list {
		keys, err := h.svc.prs.ListLinkedIssues(r.Context(), list[i].ID)
		if err != nil {
			writeAppError(w, httpapi.ErrInternal("list linked issues failed"))
			return
		}
		dtos = append(dtos, toPullRequestDTO(&list[i], keys))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"pull_requests": dtos})
}
