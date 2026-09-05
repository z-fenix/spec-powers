package issue

import (
	"context"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/notification"
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
	issues      store.IssueStore
	projects    store.ProjectStore
	users       store.UserStore
	trigger     RunTrigger
	subscribers store.SubscriberStore
	notifier    notification.Sink
	// events records the issue timeline; nil disables event recording.
	events store.IssueEventStore
}

func NewService(issues store.IssueStore, projects store.ProjectStore, users store.UserStore) *Service {
	return &Service{issues: issues, projects: projects, users: users}
}

// WithStatusStore installs the workspace status directory store; without it
// the built-in default directory applies. It returns the service for
// chaining.
func (s *Service) WithStatusStore(st store.WorkspaceStatusStore) *Service {
	s.statuses = st
	return s
}

// WithRunTrigger installs the run trigger; it returns the service for
// chaining.
func (s *Service) WithRunTrigger(t RunTrigger) *Service {
	s.trigger = t
	return s
}

// WithSubscribers installs the subscriber store; new issues then subscribe
// their creator by default.
func (s *Service) WithSubscribers(sub store.SubscriberStore) *Service {
	s.subscribers = sub
	return s
}

// WithNotifier installs a notification sink; status transitions then notify
// the issue's subscribers (except the actor).
func (s *Service) WithNotifier(n notification.Sink) *Service {
	s.notifier = n
	return s
// WithEventStore installs the timeline event store; issue creation, field
// updates and status transitions then append events.
func (s *Service) WithEventStore(e store.IssueEventStore) *Service {
	s.events = e
	return s
}

// recordEvent appends one timeline event. Recording failures fail the
// calling operation so the timeline never silently diverges from state.
func (s *Service) recordEvent(ctx context.Context, issueID, actorID, field, oldValue, newValue string) error {
	if s.events == nil {
		return nil
	}
	_, err := s.events.CreateIssueEvent(ctx, &domain.IssueEvent{
		IssueID:  issueID,
		ActorID:  actorID,
		Field:    field,
		OldValue: oldValue,
		NewValue: newValue,
	})
	if err != nil {
		return httpapi.ErrInternal("record issue event failed")
	}
	return nil
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

// directoryFor resolves the status directory governing a project: the
// project's workspace directory when a status store is installed, the
// built-in defaults otherwise.
func (s *Service) directoryFor(ctx context.Context, projectID string) ([]domain.WorkspaceStatus, error) {
	if s.statuses == nil {
		return domain.DefaultStatusDirectory(), nil
	}
	p, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("project not found")
		}
		return nil, httpapi.ErrInternal("get project failed")
	}
	dir, err := s.statuses.ListStatuses(ctx, p.WorkspaceID)
	if err != nil {
		return nil, httpapi.ErrInternal("list workspace statuses failed")
	}
	return dir, nil
}

// defaultStatus picks the initial status for a new issue: the first
// todo-category entry of the directory, falling back to the first entry.
func defaultStatus(dir []domain.WorkspaceStatus) string {
	for _, s := range dir {
		if s.Category == domain.CatTodo {
			return s.Name
		}
	}
	if len(dir) > 0 {
		return dir[0].Name
	}
	return StatusTodo
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
	dir, err := s.directoryFor(ctx, projectID)
	if err != nil {
		return nil, err
	}
	status := defaultStatus(dir)
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
	if s.subscribers != nil {
		if err := s.subscribers.AddIssueSubscriber(ctx, created.ID, userID); err != nil {
			log.Printf("issue: subscribe creator failed: %v", err)
	if err := s.recordEvent(ctx, created.ID, userID, "created", "", created.Title); err != nil {
		return nil, err
	if s.trigger != nil && in.AssigneeID != "" {
		if err := s.trigger.OnIssueAssigned(ctx, created); err != nil {
			return nil, httpapi.ErrInternal("notify assignment failed")
		}
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
	if err := s.recordFieldEvents(ctx, userID, current, saved, in); err != nil {
		return nil, err
	}
	if s.trigger != nil && in.AssigneeID != nil && *in.AssigneeID != "" && *in.AssigneeID != current.AssigneeID {
		if err := s.trigger.OnIssueAssigned(ctx, saved); err != nil {
			return nil, httpapi.ErrInternal("notify assignment failed")
		}
	}
	return saved, nil
}

// recordFieldEvents appends one event per changed tracked field. Values are
// stored in their display form; empty means unset.
func (s *Service) recordFieldEvents(ctx context.Context, userID string, before, after *domain.Issue, in UpdateInput) error {
	if s.events == nil {
		return nil
	}
	type change struct{ field, oldV, newV string }
	var changes []change
	if in.Title != nil && before.Title != after.Title {
		changes = append(changes, change{"title", before.Title, after.Title})
	}
	if in.Description != nil && before.Description != after.Description {
		changes = append(changes, change{"description", before.Description, after.Description})
	}
	if in.Priority != nil && before.Priority != after.Priority {
		changes = append(changes, change{"priority", before.Priority, after.Priority})
	}
	if in.AssigneeID != nil && before.AssigneeID != after.AssigneeID {
		changes = append(changes, change{"assignee", before.AssigneeID, after.AssigneeID})
	}
	if in.DueDate != nil && !dueEqual(before.DueDate, after.DueDate) {
		changes = append(changes, change{"due_date", formatDue(before.DueDate), formatDue(after.DueDate)})
	}
	if in.Labels != nil && !slices.Equal(before.Labels, after.Labels) {
		changes = append(changes, change{"labels", strings.Join(before.Labels, ","), strings.Join(after.Labels, ",")})
	}
	if in.Stage != nil && before.Stage != after.Stage {
		changes = append(changes, change{"stage", strconv.Itoa(before.Stage), strconv.Itoa(after.Stage)})
	}
	if in.ParentID != nil && before.ParentID != after.ParentID {
		changes = append(changes, change{"parent", before.ParentID, after.ParentID})
	}
	// Position changes are pure kanban ordering, not tracked field history.
	for _, c := range changes {
		if err := s.recordEvent(ctx, after.ID, userID, c.field, c.oldV, c.newV); err != nil {
			return err
		}
	}
	return nil
}

func dueEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func formatDue(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format("2006-01-02")
}

// GetIssueTimeline returns the issue's events, oldest first.
func (s *Service) GetIssueTimeline(ctx context.Context, userID, issueID string) ([]domain.IssueEvent, error) {
	i, err := s.requireProjectIssue(ctx, userID, issueID)
	if err != nil {
		return nil, err
	}
	if s.events == nil {
		return nil, nil
	}
	list, err := s.events.ListIssueEvents(ctx, i.ID)
	if err != nil {
		return nil, httpapi.ErrInternal("list issue events failed")
	}
	return list, nil
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
// for acceptance (Multica-compatible behavior). Subscribers are notified
// about the transition, except the actor.
func (s *Service) TransitionStatus(ctx context.Context, userID, issueID, to string) (*domain.Issue, error) {
	current, err := s.requireProjectIssue(ctx, userID, issueID)
	if err != nil {
		return nil, err
	}
	dir, err := s.directoryFor(ctx, current.ProjectID)
	if err != nil {
		return nil, err
	}
	if _, err := TransitionIn(dir, current.Status, to); err != nil {
		return nil, err
	}
	from := current.Status
	oldStatus := current.Status
	current.Status = to
	saved, err := s.issues.UpdateIssue(ctx, current)
	if err != nil {
		return nil, httpapi.ErrInternal("update issue failed")
	}
	if err := s.recordEvent(ctx, saved.ID, userID, "status", oldStatus, to); err != nil {
		return nil, err
	}
	if err := s.wakeParentIfChildrenTerminal(ctx, saved); err != nil {
		return nil, err
	}
	if s.trigger != nil {
		if err := s.trigger.OnIssueStatusChanged(ctx, saved); err != nil {
			return nil, httpapi.ErrInternal("notify status change failed")
		}
	}
	s.notifySubscribersStatusChanged(ctx, userID, saved, from, to)
	return saved, nil
}

// notifySubscribersStatusChanged is best-effort: subscriber lookups and
// delivery failures never fail the transition itself.
func (s *Service) notifySubscribersStatusChanged(ctx context.Context, actorID string, i *domain.Issue, from, to string) {
	if s.notifier == nil || s.subscribers == nil {
		return
	}
	users, err := s.subscribers.ListIssueSubscribers(ctx, i.ID)
	if err != nil {
		log.Printf("issue: list subscribers failed: %v", err)
		return
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	notification.NotifyMany(ctx, s.notifier, ids, actorID, notification.NotifyInput{
		Kind:      "status_changed",
		Title:     "Status changed on: " + i.Title,
		Body:      from + " → " + to,
		IssueID:   i.ID,
		ProjectID: i.ProjectID,
	})
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
		if !IsTerminalIn(dir, sib.Status) {
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

// StatusInput carries one directory entry for create/update.
type StatusInput struct {
	Name     string
	Category string
	Position *int
}

// ListStatuses returns the workspace status directory governing a project.
// Any project member.
func (s *Service) ListStatuses(ctx context.Context, userID, projectID string) ([]domain.WorkspaceStatus, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	return s.directoryFor(ctx, projectID)
}

// UpsertStatus creates or updates one directory entry on the project's
// workspace. Owner only; returns the full directory after the write.
func (s *Service) UpsertStatus(ctx context.Context, userID, projectID string, in StatusInput) ([]domain.WorkspaceStatus, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, httpapi.ErrInvalid("status name is required")
	}
	if !domain.IsValidStatusCategory(in.Category) {
		return nil, httpapi.ErrInvalid("unknown status category: " + in.Category)
	}
	if s.statuses == nil {
		return nil, httpapi.ErrInternal("status directory not configured")
	}
	p, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("project not found")
		}
		return nil, httpapi.ErrInternal("get project failed")
	}
	entry := &domain.WorkspaceStatus{
		WorkspaceID: p.WorkspaceID,
		Name:        in.Name,
		Category:    in.Category,
	}
	if in.Position != nil {
		entry.Position = *in.Position
	} else {
		dir, err := s.statuses.ListStatuses(ctx, p.WorkspaceID)
		if err != nil {
			return nil, httpapi.ErrInternal("list workspace statuses failed")
		}
		entry.Position = len(dir)
	}
	if _, err := s.statuses.UpsertStatus(ctx, entry); err != nil {
		return nil, httpapi.ErrInternal("upsert workspace status failed")
	}
	dir, err := s.statuses.ListStatuses(ctx, p.WorkspaceID)
	if err != nil {
		return nil, httpapi.ErrInternal("list workspace statuses failed")
	}
	return dir, nil
}

// DeleteStatus removes one directory entry from the project's workspace.
// Owner only; unknown names are 404. Returns the full directory after the
// delete.
func (s *Service) DeleteStatus(ctx context.Context, userID, projectID, name string) ([]domain.WorkspaceStatus, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	if s.statuses == nil {
		return nil, httpapi.ErrInternal("status directory not configured")
	}
	p, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("project not found")
		}
		return nil, httpapi.ErrInternal("get project failed")
	}
	if err := s.statuses.DeleteStatus(ctx, p.WorkspaceID, name); err != nil {
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("status not found")
		}
		return nil, httpapi.ErrInternal("delete workspace status failed")
	}
	dir, err := s.statuses.ListStatuses(ctx, p.WorkspaceID)
	if err != nil {
		return nil, httpapi.ErrInternal("list workspace statuses failed")
	}
	return dir, nil
}
