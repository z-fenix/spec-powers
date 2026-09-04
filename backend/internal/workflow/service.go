package workflow

import (
	"context"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
)

// Classic flow artifact kinds in phase order.
const (
	KindProposal = "proposal"
	KindSpecs    = "specs"
	KindDesign   = "design"
	KindTasks    = "tasks"
)

var kinds = map[string]bool{
	KindProposal: true, KindSpecs: true, KindDesign: true, KindTasks: true,
}

func IsValidKind(kind string) bool { return kinds[kind] }

// issueLookup and projectAccess are the slices of the issue/project stores
// the workflow domain needs.
type issueLookup interface {
	GetIssue(ctx context.Context, id string) (*domain.Issue, error)
}

type projectAccess interface {
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
}

type Service struct {
	changes   store.ChangeStore
	artifacts store.ArtifactStore
	mappings  store.TaskMappingStore
	issues    issueLookup
	projects  projectAccess
	splitter  *Splitter
	wakeups   wakeupRecorder
	creator   issueCreator
	skills    *skill.Registry
}

func NewService(changes store.ChangeStore, artifacts store.ArtifactStore, mappings store.TaskMappingStore, issues issueLookup, projects projectAccess) *Service {
	return &Service{
		changes:   changes,
		artifacts: artifacts,
		mappings:  mappings,
		issues:    issues,
		projects:  projects,
	}
}

// WithSplitter attaches the AI classic splitter; without it, StartSplit
// reports the splitter as unconfigured.
func (s *Service) WithSplitter(splitter *Splitter) *Service {
	s.splitter = splitter
	return s
}

// WithWaker attaches the wakeup recorder used by change archive to wake the
// parent issue's owner for acceptance.
func (s *Service) WithWaker(w wakeupRecorder) *Service {
	s.wakeups = w
	return s
}

// StartSplit runs the classic AI split for an issue: it creates the change,
// generates the four artifacts, and creates the staged sub-issues. The
// caller must be a project member.
func (s *Service) StartSplit(ctx context.Context, userID, issueID string) (*domain.Change, []domain.TaskMapping, error) {
	i, err := s.issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, nil, httpapi.ErrInternal("get issue failed")
	}
	if err := s.requireProjectRole(ctx, userID, i.ProjectID); err != nil {
		return nil, nil, err
	}
	if s.splitter == nil {
		return nil, nil, httpapi.ErrInvalid("splitter is not configured")
	}
	change, err := s.splitter.Run(ctx, userID, issueID)
	if err != nil {
		return nil, nil, err
	}
	tasks, err := s.mappings.ListTaskMappings(ctx, change.ID)
	if err != nil {
		return change, nil, httpapi.ErrInternal("list task mappings failed")
	}
	return change, tasks, nil
}

// requireChangeRole loads the change and enforces project-level access;
// unknown changes/projects are 404 and non-members 403 (same semantics as
// the other domains).
func (s *Service) requireChangeRole(ctx context.Context, userID, changeID string) (*domain.Change, error) {
	c, err := s.changes.GetChange(ctx, changeID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("change not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get change failed")
	}
	if err := s.requireProjectRole(ctx, userID, c.ProjectID); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) requireProjectRole(ctx context.Context, userID, projectID string) error {
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
	_ = pm
	return nil
}

func (s *Service) GetChange(ctx context.Context, userID, changeID string) (*domain.Change, error) {
	return s.requireChangeRole(ctx, userID, changeID)
}

// GetChangeByIssue returns the change running for an issue.
func (s *Service) GetChangeByIssue(ctx context.Context, userID, issueID string) (*domain.Change, error) {
	i, err := s.issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get issue failed")
	}
	if err := s.requireProjectRole(ctx, userID, i.ProjectID); err != nil {
		return nil, err
	}
	c, err := s.changes.GetChangeByIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("change not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get change failed")
	}
	return c, nil
}

// ListArtifacts returns the change's latest artifact per kind, in phase
// order (proposal, specs, design, tasks).
func (s *Service) ListArtifacts(ctx context.Context, userID, changeID string) ([]domain.Artifact, error) {
	if _, err := s.requireChangeRole(ctx, userID, changeID); err != nil {
		return nil, err
	}
	list, err := s.artifacts.ListArtifacts(ctx, changeID)
	if err != nil {
		return nil, httpapi.ErrInternal("list artifacts failed")
	}
	return list, nil
}

// GetArtifact returns one artifact kind; version <= 0 selects the latest.
func (s *Service) GetArtifact(ctx context.Context, userID, changeID, kind string, version int) (*domain.Artifact, error) {
	if !IsValidKind(kind) {
		return nil, httpapi.ErrInvalid("unknown artifact kind: " + kind)
	}
	if _, err := s.requireChangeRole(ctx, userID, changeID); err != nil {
		return nil, err
	}
	a, err := s.artifacts.GetArtifact(ctx, changeID, kind, version)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("artifact not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get artifact failed")
	}
	return a, nil
}

// ListTaskMappings returns the tasks-artifact entries mapped to sub-issues.
func (s *Service) ListTaskMappings(ctx context.Context, userID, changeID string) ([]domain.TaskMapping, error) {
	if _, err := s.requireChangeRole(ctx, userID, changeID); err != nil {
		return nil, err
	}
	list, err := s.mappings.ListTaskMappings(ctx, changeID)
	if err != nil {
		return nil, httpapi.ErrInternal("list task mappings failed")
	}
	return list, nil
}
