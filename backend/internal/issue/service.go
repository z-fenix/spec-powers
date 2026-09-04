package issue

import (
	"context"
	"strings"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// RunTrigger is notified when an issue's assignment or status changes and
// when a parent issue is woken by its children reaching terminal states.
// The agent runtime implements it to enqueue runs.
type RunTrigger interface {
	OnIssueAssigned(ctx context.Context, i *domain.Issue) error
	OnIssueStatusChanged(ctx context.Context, i *domain.Issue) error
	OnParentWakeup(ctx context.Context, parent *domain.Issue) error
}

type Service struct {
	issues   store.IssueStore
	projects store.ProjectStore
	users    store.UserStore
	trigger  RunTrigger
}

func NewService(issues store.IssueStore, projects store.ProjectStore, users store.UserStore) *Service {
	return &Service{issues: issues, projects: projects, users: users}
}

// WithRunTrigger installs the run trigger; it returns the service for
// chaining.
func (s *Service) WithRunTrigger(t RunTrigger) *Service {
	s.trigger = t
	return s
}

type CreateInput struct {
	Title       string
	Description string
	Priority    string
	AssigneeID  string
	DueDate     *time.Time
	Labels      []string
	ParentID    string
	Stage       int
}

type UpdateInput struct {
	Title       *string
	Description *string
	Priority    *string
	AssigneeID  *string
	DueDate     *time.Time
	Labels      []string
	ParentID    *string
	Stage       *int
	Position    *int
}

func (s *Service) requireProjectIssue(ctx context.Context, userID, issueID string) (*domain.Issue, error) {
	i, err := s.issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get issue failed")
	}
	if err := s.requireProjectRole(ctx, userID, i.ProjectID, "member"); err != nil {
		return nil, err
	}
	return i, nil
}

// requireProjectRole enforces project-level access; unknown projects are 404
// and non-members 403 (same semantics as the project domain).
func (s *Service) requireProjectRole(ctx context.Context, userID, projectID, minRole string) error {
	if _, err := s.projects.GetProject(ctx, projectID); err != nil {
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project failed")
	}
	pm, err := s.projects.GetProjectMember(ctx, projectID, userID)
	if err == store.ErrNotFound {
		return httpapi.ErrForbidden("not a project member")
	}
	if err != nil {
		return httpapi.ErrInternal("get project member failed")
	}
	if minRole == "owner" && pm.Role != "owner" {
		return httpapi.ErrForbidden("owner role required")
	}
	return nil
}

func validatePriority(p string) error {
	if !IsValidPriority(p) {
		return httpapi.ErrInvalid("unknown priority: " + p)
	}
	return nil
}

func (s *Service) validateAssignee(ctx context.Context, assigneeID string) error {
	if assigneeID == "" {
		return nil
	}
	if _, err := s.users.GetUser(ctx, assigneeID); err == store.ErrNotFound {
		return httpapi.ErrNotFound("assignee not found")
	} else if err != nil {
		return httpapi.ErrInternal("lookup assignee failed")
	}
	return nil
}

func (s *Service) CreateIssue(ctx context.Context, userID, projectID string, in CreateInput) (*domain.Issue, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, httpapi.ErrInvalid("issue title is required")
	}
	if in.Priority == "" {
		in.Priority = PriorityNone
	}
	if err := validatePriority(in.Priority); err != nil {
		return nil, err
	}
	if err := s.validateAssignee(ctx, in.AssigneeID); err != nil {
		return nil, err
	}
	status := StatusTodo
	if in.ParentID != "" {
		parent, err := s.issues.GetIssue(ctx, in.ParentID)
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("parent issue not found")
		}
		if err != nil {
			return nil, httpapi.ErrInternal("get parent issue failed")
		}
		if parent.ProjectID != projectID {
			return nil, httpapi.ErrInvalid("parent issue belongs to another project")
		}
	}
	i := &domain.Issue{
		ProjectID:   projectID,
		ParentID:    in.ParentID,
		Title:       in.Title,
		Description: in.Description,
		Status:      status,
		Priority:    in.Priority,
		AssigneeID:  in.AssigneeID,
		DueDate:     in.DueDate,
		Labels:      in.Labels,
		Stage:       in.Stage,
		CreatedBy:   userID,
	}
	pos, err := s.issues.NextIssuePosition(ctx, projectID, in.ParentID, in.Stage)
	if err != nil {
		return nil, httpapi.ErrInternal("next position failed")
	}
	i.Position = pos
	created, err := s.issues.CreateIssue(ctx, i)
	if err != nil {
		return nil, httpapi.ErrInternal("create issue failed")
	}
	return created, nil
}

func (s *Service) GetIssue(ctx context.Context, userID, issueID string) (*domain.Issue, error) {
	return s.requireProjectIssue(ctx, userID, issueID)
}

func (s *Service) ListIssues(ctx context.Context, userID, projectID string, filter store.IssueFilter) ([]domain.Issue, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	list, err := s.issues.ListIssues(ctx, projectID, filter)
	if err != nil {
		return nil, httpapi.ErrInternal("list issues failed")
	}
	return list, nil
}

func (s *Service) UpdateIssue(ctx context.Context, userID, issueID string, in UpdateInput) (*domain.Issue, error) {
	current, err := s.requireProjectIssue(ctx, userID, issueID)
	if err != nil {
		return nil, err
	}
	updated := *current
	if in.Title != nil {
		if strings.TrimSpace(*in.Title) == "" {
			return nil, httpapi.ErrInvalid("issue title is required")
		}
		updated.Title = *in.Title
	}
	if in.Description != nil {
		updated.Description = *in.Description
	}
	if in.Priority != nil {
		if err := validatePriority(*in.Priority); err != nil {
			return nil, err
		}
		updated.Priority = *in.Priority
	}
	if in.AssigneeID != nil {
		if err := s.validateAssignee(ctx, *in.AssigneeID); err != nil {
			return nil, err
		}
		updated.AssigneeID = *in.AssigneeID
	}
	if in.DueDate != nil {
		updated.DueDate = in.DueDate
	}
	if in.Labels != nil {
		updated.Labels = in.Labels
	}
	if in.Stage != nil {
		updated.Stage = *in.Stage
	}
	if in.Position != nil {
		updated.Position = *in.Position
	}
	if in.ParentID != nil {
		if err := s.validateNewParent(ctx, current, *in.ParentID); err != nil {
			return nil, err
		}
		updated.ParentID = *in.ParentID
	}
	saved, err := s.issues.UpdateIssue(ctx, &updated)
	if err != nil {
		return nil, httpapi.ErrInternal("update issue failed")
	}
	if s.trigger != nil && in.AssigneeID != nil && *in.AssigneeID != "" && *in.AssigneeID != current.AssigneeID {
		if err := s.trigger.OnIssueAssigned(ctx, saved); err != nil {
			return nil, httpapi.ErrInternal("notify assignment failed")
		}
	}
	return saved, nil
}

// validateNewParent checks a parent move: parent must exist in the same
// project, must not be the issue itself, and must not be one of its
// descendants (which would create a cycle).
func (s *Service) validateNewParent(ctx context.Context, current *domain.Issue, newParentID string) error {
	if newParentID == "" {
		return nil
	}
	if newParentID == current.ID {
		return httpapi.ErrInvalid("issue cannot be its own parent")
	}
	parent, err := s.issues.GetIssue(ctx, newParentID)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("parent issue not found")
	}
	if err != nil {
		return httpapi.ErrInternal("get parent issue failed")
	}
	if parent.ProjectID != current.ProjectID {
		return httpapi.ErrInvalid("parent issue belongs to another project")
	}
	ancestor := parent
	for ancestor.ParentID != "" {
		if ancestor.ParentID == current.ID {
			return httpapi.ErrInvalid("parent move would create a cycle")
		}
		ancestor, err = s.issues.GetIssue(ctx, ancestor.ParentID)
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("parent issue not found")
		}
		if err != nil {
			return httpapi.ErrInternal("get parent issue failed")
		}
	}
	return nil
}

func (s *Service) DeleteIssue(ctx context.Context, userID, issueID string) error {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return err
	}
	if err := s.issues.DeleteIssue(ctx, issueID); err != nil {
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("issue not found")
		}
		return httpapi.ErrInternal("delete issue failed")
	}
	return nil
}

// GetChildren returns an issue's direct sub-issues, ordered by stage then
// position.
func (s *Service) GetChildren(ctx context.Context, userID, issueID string) ([]domain.Issue, error) {
	parent, err := s.requireProjectIssue(ctx, userID, issueID)
	if err != nil {
		return nil, err
	}
	kids, err := s.issues.ListIssues(ctx, parent.ProjectID, store.IssueFilter{ParentID: &parent.ID})
	if err != nil {
		return nil, httpapi.ErrInternal("list children failed")
	}
	for a := 1; a < len(kids); a++ {
		for b := a; b > 0 && (kids[b].Stage < kids[b-1].Stage ||
			(kids[b].Stage == kids[b-1].Stage && kids[b].Position < kids[b-1].Position)); b-- {
			kids[b], kids[b-1] = kids[b-1], kids[b]
		}
	}
	return kids, nil
}

// TransitionStatus moves an issue through the kanban state machine. When the
// issue is a child and reaches a terminal state while every sibling is also
// terminal, a wakeup is recorded on the parent so its assignee can be woken
// for acceptance (Multica-compatible behavior).
func (s *Service) TransitionStatus(ctx context.Context, userID, issueID, to string) (*domain.Issue, error) {
	current, err := s.requireProjectIssue(ctx, userID, issueID)
	if err != nil {
		return nil, err
	}
	if _, err := Transition(current.Status, to); err != nil {
		return nil, err
	}
	current.Status = to
	saved, err := s.issues.UpdateIssue(ctx, current)
	if err != nil {
		return nil, httpapi.ErrInternal("update issue failed")
	}
	if err := s.wakeParentIfChildrenTerminal(ctx, saved); err != nil {
		return nil, err
	}
	if s.trigger != nil {
		if err := s.trigger.OnIssueStatusChanged(ctx, saved); err != nil {
			return nil, httpapi.ErrInternal("notify status change failed")
		}
	}
	return saved, nil
}

func (s *Service) wakeParentIfChildrenTerminal(ctx context.Context, child *domain.Issue) error {
	if child.ParentID == "" || !IsTerminal(child.Status) {
		return nil
	}
	siblings, err := s.issues.ListIssues(ctx, child.ProjectID, store.IssueFilter{ParentID: &child.ParentID})
	if err != nil {
		return httpapi.ErrInternal("list sibling issues failed")
	}
	for _, sib := range siblings {
		if !IsTerminal(sib.Status) {
			return nil
		}
	}
	if err := s.issues.CreateIssueWakeup(ctx, child.ParentID, child.ID); err != nil {
		return httpapi.ErrInternal("record parent wakeup failed")
	}
	if s.trigger != nil {
		parent, err := s.issues.GetIssue(ctx, child.ParentID)
		if err != nil {
			return httpapi.ErrInternal("get parent issue failed")
		}
		if err := s.trigger.OnParentWakeup(ctx, parent); err != nil {
			return httpapi.ErrInternal("notify parent wakeup failed")
		}
	}
	return nil
}
