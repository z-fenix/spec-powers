package agent

import (
	"context"
	"net/http"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
	"specpowers/backend/internal/workflow"
)

// FlowDriver is the executor's view of the classic workflow: one change per
// issue, phase skills, versioned artifacts and guard-checked advances.
type FlowDriver interface {
	EnsureChange(ctx context.Context, userID, issueID string) (*domain.Change, error)
	PhaseSkill(ctx context.Context, userID string, change *domain.Change) (*skill.Skill, error)
	WriteArtifact(ctx context.Context, userID string, change *domain.Change, kind, content string) (*domain.Artifact, error)
	AdvancePhase(ctx context.Context, userID string, change *domain.Change) (*domain.Change, error)
	SubmitVerify(ctx context.Context, userID string, change *domain.Change, content string) (*domain.Artifact, error)
	Archive(ctx context.Context, userID string, change *domain.Change) (*domain.Change, error)
}

// WorkflowFlow adapts the workflow service to the executor's FlowDriver.
type WorkflowFlow struct {
	svc *workflow.Service
}

func NewWorkflowFlow(svc *workflow.Service) *WorkflowFlow {
	return &WorkflowFlow{svc: svc}
}

var _ FlowDriver = (*WorkflowFlow)(nil)

// EnsureChange returns the issue's change, creating a bare proposal-phase
// one when the issue has none.
func (w *WorkflowFlow) EnsureChange(ctx context.Context, userID, issueID string) (*domain.Change, error) {
	c, err := w.svc.GetChangeByIssue(ctx, userID, issueID)
	if err == nil {
		return c, nil
	}
	if appErr, ok := err.(*httpapi.AppError); !ok || appErr.Status != http.StatusNotFound {
		return nil, err
	}
	c, _, err = w.svc.StartChange(ctx, userID, issueID, true)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (w *WorkflowFlow) PhaseSkill(ctx context.Context, userID string, change *domain.Change) (*skill.Skill, error) {
	return w.svc.NextSkill(ctx, userID, change.ID)
}

func (w *WorkflowFlow) WriteArtifact(ctx context.Context, userID string, change *domain.Change, kind, content string) (*domain.Artifact, error) {
	return w.svc.WriteArtifact(ctx, userID, change.ID, kind, content)
}

func (w *WorkflowFlow) AdvancePhase(ctx context.Context, userID string, change *domain.Change) (*domain.Change, error) {
	c, _, err := w.svc.AdvancePhase(ctx, userID, change.ID)
	return c, err
}

func (w *WorkflowFlow) SubmitVerify(ctx context.Context, userID string, change *domain.Change, content string) (*domain.Artifact, error) {
	a, _, err := w.svc.SubmitVerifyReport(ctx, userID, change.ID, content)
	return a, err
}

func (w *WorkflowFlow) Archive(ctx context.Context, userID string, change *domain.Change) (*domain.Change, error) {
	return w.svc.Archive(ctx, userID, change.ID)
}

// StoreAgentAccess adapts the agent store to the workflow service's agent
// lookup: a user with an agents row is an agent identity.
type StoreAgentAccess struct {
	Agents store.AgentStore
}

func (s StoreAgentAccess) IsAgent(ctx context.Context, userID string) bool {
	if userID == "" {
		return false
	}
	_, err := s.Agents.GetAgent(ctx, userID)
	return err == nil
}
