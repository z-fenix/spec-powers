package pr

import (
	"context"
	"strings"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/store"
)

// Pull request states.
const (
	StateOpen   = "open"
	StateMerged = "merged"
	StateClosed = "closed"
)

var validStates = map[string]bool{StateOpen: true, StateMerged: true, StateClosed: true}

// Projects is the project access the service needs: role checks plus the
// issue-key prefix lookup. *postgres.ProjectStore implements it.
type Projects interface {
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
	GetProjectByKey(ctx context.Context, workspaceID, key string) (*domain.Project, error)
}

// Issues is the issue access the service needs: resolving keys to issues and
// applying close intents. *postgres.IssueStore implements it.
type Issues interface {
	GetIssue(ctx context.Context, id string) (*domain.Issue, error)
	GetIssueByNumber(ctx context.Context, projectID string, number int64) (*domain.Issue, error)
	UpdateIssue(ctx context.Context, i *domain.Issue) (*domain.Issue, error)
}

type Service struct {
	prs      store.PullRequestStore
	issues   Issues
	projects Projects
	// events records close-intent status changes on the issue timeline;
	// nil disables event recording.
	events store.IssueEventStore
}

func NewService(prs store.PullRequestStore, issues Issues, projects Projects) *Service {
	return &Service{prs: prs, issues: issues, projects: projects}
}

// WithEventStore installs the timeline event store; close-intent auto-closes
// then append a status event to the issue's timeline.
func (s *Service) WithEventStore(e store.IssueEventStore) *Service {
	s.events = e
	return s
}

type UpsertInput struct {
	Repo       string
	Number     int64
	Title      string
	Body       string
	HeadBranch string
	// State optionally sets the PR state ("open", "merged", "closed");
	// empty means open. A transition to merged applies close intents.
	State string
}

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

// resolveIssueKey maps an issue key ("SP-44") to an issue within the PR
// project's workspace. Keys that reference no known project or issue return
// a nil issue — unresolvable references are skipped, not errors.
func (s *Service) resolveIssueKey(ctx context.Context, wsProject *domain.Project, key string) (*domain.Issue, error) {
	prefix, number := SplitIssueKey(key)
	if prefix == "" || number <= 0 {
		return nil, nil
	}
	keyProject, err := s.projects.GetProjectByKey(ctx, wsProject.WorkspaceID, prefix)
	if err == store.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, httpapi.ErrInternal("resolve project key failed")
	}
	i, err := s.issues.GetIssueByNumber(ctx, keyProject.ID, number)
	if err == store.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, httpapi.ErrInternal("resolve issue number failed")
	}
	return i, nil
}

// UpsertPullRequest records or refreshes a PR and (re)links every issue its
// title, body and branch name reference by key. When the resulting state is
// merged and the PR was not merged before, close intents are applied.
// It returns the PR and the issue keys now linked to it.
func (s *Service) UpsertPullRequest(ctx context.Context, userID, projectID string, in UpsertInput) (*domain.PullRequest, []string, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, nil, err
	}
	if in.Number <= 0 {
		return nil, nil, httpapi.ErrInvalid("pull request number must be positive")
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, nil, httpapi.ErrInvalid("pull request title is required")
	}
	state := in.State
	if state == "" {
		state = StateOpen
	}
	if !validStates[state] {
		return nil, nil, httpapi.ErrInvalid("unknown pull request state: " + state)
	}
	project, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, nil, httpapi.ErrInternal("get project failed")
	}

	previous, err := s.currentPRState(ctx, projectID, in.Repo, in.Number)
	if err != nil {
		return nil, nil, err
	}
	pr, err := s.prs.UpsertPullRequest(ctx, &domain.PullRequest{
		ProjectID:  projectID,
		Repo:       in.Repo,
		Number:     in.Number,
		Title:      in.Title,
		Body:       in.Body,
		HeadBranch: in.HeadBranch,
		State:      state,
		CreatedBy:  userID,
	})
	if err != nil {
		return nil, nil, httpapi.ErrInternal("upsert pull request failed")
	}
	linked, err := s.linkReferencedIssues(ctx, project, pr)
	if err != nil {
		return nil, nil, err
	}
	if state == StateMerged && previous != StateMerged {
		if err := s.applyCloseIntents(ctx, userID, pr); err != nil {
			return nil, nil, err
		}
	}
	return pr, linked, nil
}

// currentPRState returns the stored state of the PR, or "" when the PR is
// new. ErrConflict (a concurrent upsert) surfaces as-is.
func (s *Service) currentPRState(ctx context.Context, projectID, repo string, number int64) (string, error) {
	stored, err := s.prs.GetPullRequestByProjectNumber(ctx, projectID, repo, number)
	if err == store.ErrNotFound {
		return "", nil
	}
	if err != nil {
		return "", httpapi.ErrInternal("get pull request failed")
	}
	return stored.State, nil
}

// linkReferencedIssues links every issue referenced by the PR's title, body
// and head branch and returns the linked issue keys.
func (s *Service) linkReferencedIssues(ctx context.Context, project *domain.Project, pr *domain.PullRequest) ([]string, error) {
	keys := ExtractIssueKeys(pr.Title + "\n" + pr.Body + "\n" + pr.HeadBranch)
	var linked []string
	for _, key := range keys {
		i, err := s.resolveIssueKey(ctx, project, key)
		if err != nil {
			return nil, err
		}
		if i == nil {
			continue
		}
		if err := s.prs.LinkIssue(ctx, pr.ID, i.ID); err != nil {
			return nil, httpapi.ErrInternal("link issue failed")
		}
		linked = append(linked, key)
	}
	return linked, nil
}

// applyCloseIntents closes every issue the PR's title/body asks to close
// with a close-intent keyword. Terminal issues are left untouched.
func (s *Service) applyCloseIntents(ctx context.Context, userID string, pr *domain.PullRequest) error {
	project, err := s.projects.GetProject(ctx, pr.ProjectID)
	if err != nil {
		return httpapi.ErrInternal("get project failed")
	}
	for _, key := range ParseCloseIntents(pr.Title + "\n" + pr.Body) {
		i, err := s.resolveIssueKey(ctx, project, key)
		if err != nil {
			return err
		}
		if i == nil || issue.IsTerminal(i.Status) {
			continue
		}
		closed := *i
		closed.Status = issue.StatusDone
		if _, err := s.issues.UpdateIssue(ctx, &closed); err != nil {
			return httpapi.ErrInternal("close issue failed")
		}
		if err := s.recordEvent(ctx, i.ID, userID, "status", i.Status, issue.StatusDone); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePullRequestState moves a PR's state. Moving to merged stamps
// merged_at and applies close intents; it is a no-op when already merged.
func (s *Service) UpdatePullRequestState(ctx context.Context, userID, prID, state string) (*domain.PullRequest, error) {
	if err := s.requirePRMember(ctx, userID, prID); err != nil {
		return nil, err
	}
	if !validStates[state] {
		return nil, httpapi.ErrInvalid("unknown pull request state: " + state)
	}
	pr, err := s.prs.GetPullRequest(ctx, prID)
	if err != nil {
		return nil, httpapi.ErrInternal("get pull request failed")
	}
	var mergedAt *time.Time
	if state == StateMerged && pr.State != StateMerged {
		now := time.Now()
		mergedAt = &now
	}
	pr, err = s.prs.UpdatePullRequestState(ctx, prID, state, mergedAt)
	if err != nil {
		return nil, httpapi.ErrInternal("update pull request failed")
	}
	if mergedAt != nil {
		if err := s.applyCloseIntents(ctx, userID, pr); err != nil {
			return nil, err
		}
	}
	return pr, nil
}

// requirePRMember checks the caller is a member of the PR's project.
func (s *Service) requirePRMember(ctx context.Context, userID, prID string) error {
	pr, err := s.prs.GetPullRequest(ctx, prID)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("pull request not found")
	}
	if err != nil {
		return httpapi.ErrInternal("get pull request failed")
	}
	return s.requireProjectRole(ctx, userID, pr.ProjectID, "member")
}

// GetPullRequest returns one PR with its linked issue keys.
func (s *Service) GetPullRequest(ctx context.Context, userID, prID string) (*domain.PullRequest, []string, error) {
	if err := s.requirePRMember(ctx, userID, prID); err != nil {
		return nil, nil, err
	}
	pr, err := s.prs.GetPullRequest(ctx, prID)
	if err != nil {
		return nil, nil, httpapi.ErrInternal("get pull request failed")
	}
	keys, err := s.prs.ListLinkedIssues(ctx, prID)
	if err != nil {
		return nil, nil, httpapi.ErrInternal("list linked issues failed")
	}
	return pr, keys, nil
}

// ListIssuePullRequests returns the PRs linked to an issue, newest first.
func (s *Service) ListIssuePullRequests(ctx context.Context, userID, issueID string) ([]domain.PullRequest, error) {
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
	list, err := s.prs.ListPullRequestsForIssue(ctx, issueID)
	if err != nil {
		return nil, httpapi.ErrInternal("list issue pull requests failed")
	}
	return list, nil
}
