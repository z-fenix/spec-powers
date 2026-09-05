package issue

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type Handler struct {
	svc    *Service
	tokens *auth.TokenService
	// collab is the comment/attachment/metadata subrouter mounted under
	// /{issueID}; nil in tests that don't exercise collaboration.
	collab http.Handler
	// pullRequests is the linked-PR listing subrouter mounted under
	// /{issueID}/pullrequests; nil in tests that don't exercise PRs.
	pullRequests http.Handler
	// properties is the issue property-value subrouter mounted under
	// /{issueID}/properties; nil in tests that don't exercise properties.
	properties http.Handler
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

// WithCollab attaches the collaboration subrouter served under /{issueID}.
func (h *Handler) WithCollab(c http.Handler) *Handler {
	h.collab = c
	return h
}

// WithPullRequests attaches the linked-PR listing subrouter served under
// /{issueID}/pullrequests.
func (h *Handler) WithPullRequests(p http.Handler) *Handler {
	h.pullRequests = p
	return h
}

// WithProperties attaches the property-value subrouter served under
// /{issueID}/properties.
func (h *Handler) WithProperties(p http.Handler) *Handler {
	h.properties = p
	return h
}

type issueDTO struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	ParentID    string   `json:"parent_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	AssigneeID  string   `json:"assignee_id"`
	DueDate     string   `json:"due_date"`
	Labels      []string `json:"labels"`
	Stage       int      `json:"stage"`
	Position    int      `json:"position"`
	CreatedBy   string   `json:"created_by"`
}

func toIssueDTO(i *domain.Issue) issueDTO {
	due := ""
	if i.DueDate != nil {
		due = i.DueDate.Format("2006-01-02")
	}
	labels := i.Labels
	if labels == nil {
		labels = []string{}
	}
	return issueDTO{
		ID: i.ID, ProjectID: i.ProjectID, ParentID: i.ParentID,
		Title: i.Title, Description: i.Description, Status: i.Status,
		Priority: i.Priority, AssigneeID: i.AssigneeID, DueDate: due,
		Labels: labels, Stage: i.Stage, Position: i.Position, CreatedBy: i.CreatedBy,
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Route("/{issueID}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Patch("/", h.update)
		r.Delete("/", h.remove)
		r.Post("/status", h.transition)
		r.Get("/children", h.children)
		if h.properties != nil {
			r.Mount("/properties", h.properties)
		}
		r.Get("/events", h.events)
		if h.pullRequests != nil {
			r.Mount("/pullrequests", h.pullRequests)
		}
		if h.collab != nil {
			r.Mount("/", h.collab)
		}
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

// parseDueDate accepts "YYYY-MM-DD" or "" (nil). Any other shape is a 400.
func parseDueDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, httpapi.ErrInvalid("due_date must be YYYY-MM-DD")
	}
	return &d, nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Priority    string   `json:"priority"`
		AssigneeID  string   `json:"assignee_id"`
		DueDate     string   `json:"due_date"`
		Labels      []string `json:"labels"`
		ParentID    string   `json:"parent_id"`
		Stage       int      `json:"stage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	due, err := parseDueDate(req.DueDate)
	if err != nil {
		writeAppError(w, err)
		return
	}
	i, err := h.svc.CreateIssue(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), CreateInput{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
		DueDate:     due,
		Labels:      req.Labels,
		ParentID:    req.ParentID,
		Stage:       req.Stage,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"issue": toIssueDTO(i)})
}

// parseListFilter reads list query params: status, stage, parent ("root"
// selects root issues only) and the q search keyword. Unknown values are
// silently ignored except stage, which must be numeric.
func parseListFilter(r *http.Request) (store.IssueFilter, error) {
	var filter store.IssueFilter
	q := r.URL.Query()
	if s := q.Get("status"); s != "" {
		filter.Status = s
	}
	if s := q.Get("stage"); s != "" {
		stage, err := strconv.Atoi(s)
		if err != nil {
			return filter, httpapi.ErrInvalid("stage must be an integer")
		}
		filter.Stage = &stage
	}
	if s := q.Get("parent"); s != "" {
		parent := s
		if s == "root" {
			parent = ""
		}
		filter.ParentID = &parent
	}
	if s := q.Get("q"); s != "" {
		filter.Query = s
	}
	return filter, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListFilter(r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	list, err := h.svc.ListIssues(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "projectID"), filter)
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]issueDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toIssueDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"issues": dtos})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	i, err := h.svc.GetIssue(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"issue": toIssueDTO(i)})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       *string   `json:"title"`
		Description *string   `json:"description"`
		Priority    *string   `json:"priority"`
		AssigneeID  *string   `json:"assignee_id"`
		DueDate     *string   `json:"due_date"`
		Labels      *[]string `json:"labels"`
		ParentID    *string   `json:"parent_id"`
		Stage       *int      `json:"stage"`
		Position    *int      `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	in := UpdateInput{
		Title: req.Title, Description: req.Description, Priority: req.Priority,
		AssigneeID: req.AssigneeID, ParentID: req.ParentID,
		Stage: req.Stage, Position: req.Position,
	}
	if req.Labels != nil {
		in.Labels = *req.Labels
	}
	if req.DueDate != nil {
		due, err := parseDueDate(*req.DueDate)
		if err != nil {
			writeAppError(w, err)
			return
		}
		in.DueDate = due
	}
	i, err := h.svc.UpdateIssue(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"), in)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"issue": toIssueDTO(i)})
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteIssue(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	i, err := h.svc.TransitionStatus(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"), req.Status)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"issue": toIssueDTO(i)})
}

func (h *Handler) children(w http.ResponseWriter, r *http.Request) {
	kids, err := h.svc.GetChildren(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]issueDTO, 0, len(kids))
	for i := range kids {
		dtos = append(dtos, toIssueDTO(&kids[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"issues": dtos})
}

type issueEventDTO struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	ActorID   string `json:"actor_id"`
	Field     string `json:"field"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
	CreatedAt string `json:"created_at"`
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.GetIssueTimeline(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "issueID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]issueEventDTO, 0, len(list))
	for _, e := range list {
		dtos = append(dtos, issueEventDTO{
			ID: e.ID, IssueID: e.IssueID, ActorID: e.ActorID, Field: e.Field,
			OldValue: e.OldValue, NewValue: e.NewValue,
			CreatedAt: e.CreatedAt.Format(time.RFC3339),
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"events": dtos})
}
