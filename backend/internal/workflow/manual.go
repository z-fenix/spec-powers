package workflow

import (
	"context"
	"strings"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// WithCreator attaches the issue service used to create sub-issues when a
// tasks artifact is written manually.
func (s *Service) WithCreator(c issueCreator) *Service {
	s.creator = c
	return s
}

// WriteArtifact stores a new version of one artifact kind for the change,
// letting the agent-driven skill flow produce artifacts without the LLM
// splitter. Writing the tasks kind parses the tasks JSON and creates the
// sub-issues with task mappings, mirroring the splitter's tasks phase.
func (s *Service) WriteArtifact(ctx context.Context, userID, changeID, kind, content string) (*domain.Artifact, error) {
	if !IsValidKind(kind) {
		return nil, httpapi.ErrInvalid("unknown artifact kind: " + kind)
	}
	if strings.TrimSpace(content) == "" {
		return nil, httpapi.ErrInvalid("content is required")
	}
	c, err := s.requireChangeRole(ctx, userID, changeID)
	if err != nil {
		return nil, err
	}
	if c.Status != "active" {
		return nil, httpapi.ErrConflict("change is not active")
	}
	a, err := s.artifacts.CreateArtifact(ctx, &domain.Artifact{
		ChangeID:  c.ID,
		Kind:      kind,
		Content:   content,
		CreatedBy: userID,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("save artifact failed")
	}
	if kind == KindTasks {
		if err := s.bindTaskSubIssues(ctx, userID, c, a, content); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// bindTaskSubIssues creates the parsed sub-issues under the change's parent
// issue and binds them to the tasks artifact via task mappings.
func (s *Service) bindTaskSubIssues(ctx context.Context, userID string, c *domain.Change, tasksArtifact *domain.Artifact, content string) error {
	if s.creator == nil {
		return httpapi.ErrInternal("issue creator is not configured")
	}
	parent, err := s.issues.GetIssue(ctx, c.IssueID)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return httpapi.ErrInternal("get issue failed")
	}
	if err := bindTaskSubIssuesTo(ctx, s.creator, s.mappings, userID, parent, c, tasksArtifact, content); err != nil {
		return err
	}
	return nil
}

// StartChange opens a change for an issue. With manual=true it creates a
// bare proposal-phase change without the AI splitter, so the agent-driven
// skill flow (brainstorm → write-plan → …) can produce the artifacts
// itself; otherwise it runs the classic AI split.
func (s *Service) StartChange(ctx context.Context, userID, issueID string, manual bool) (*domain.Change, []domain.TaskMapping, error) {
	if !manual {
		return s.StartSplit(ctx, userID, issueID)
	}
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
	if _, err := s.changes.GetChangeByIssue(ctx, issueID); err == nil {
		return nil, nil, httpapi.ErrConflict("change already exists for this issue")
	} else if err != store.ErrNotFound {
		return nil, nil, httpapi.ErrInternal("get change failed")
	}
	c, err := s.changes.CreateChange(ctx, &domain.Change{
		ProjectID: i.ProjectID,
		IssueID:   issueID,
		Phase:     KindProposal,
		Status:    "active",
		CreatedBy: userID,
	})
	if err != nil {
		return nil, nil, httpapi.ErrInternal("create change failed")
	}
	return c, nil, nil
}
