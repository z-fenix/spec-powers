package workflow

import (
	"context"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// wakeupRecorder records a parent-issue wakeup so the parent's owner can be
// woken for acceptance (Multica-compatible linkage).
type wakeupRecorder interface {
	CreateIssueWakeup(ctx context.Context, issueID, childIssueID string) error
}

// parentWakeupHook is invoked after an archive records a parent wakeup, so
// the parent's owner gets a notification (human) or a run (agent). It
// matches issue.RunTrigger's OnParentWakeup without importing the package.
type parentWakeupHook interface {
	OnParentWakeup(ctx context.Context, parent *domain.Issue) error
}

// GuardReport is the gate evaluation for a change.
type GuardReport struct {
	ChangeID     string   `json:"change_id"`
	Phase        string   `json:"phase"`
	NextPhase    string   `json:"next_phase"`
	PhaseLegal   bool     `json:"phase_legal"`
	HandoffFresh bool     `json:"handoff_fresh"`
	VerifyPassed bool     `json:"verify_passed"`
	CanAdvance   bool     `json:"can_advance"`
	CanArchive   bool     `json:"can_archive"`
	Reasons      []string `json:"reasons"`
}

// NextPhase returns the phase after the given one in the classic order.
func NextPhase(phase string) (string, bool) {
	order := phaseOrder()
	for i, k := range order {
		if k == phase {
			if i+1 < len(order) {
				return order[i+1], true
			}
			return "", false
		}
	}
	return "", false
}

// GuardStatus evaluates the change's gates without mutating anything.
func (s *Service) GuardStatus(ctx context.Context, userID, changeID string) (*GuardReport, error) {
	c, err := s.requireChangeRole(ctx, userID, changeID)
	if err != nil {
		return nil, err
	}
	legal, reasons := s.phaseLegal(ctx, c)
	fresh, freshReason := s.handoffFresh(ctx, c)
	verifyPassed, verifyReason := s.verifyPassed(ctx, c.ID)
	active := c.Status == "active"

	next, has := NextPhase(c.Phase)
	report := &GuardReport{
		ChangeID:     c.ID,
		Phase:        c.Phase,
		NextPhase:    next,
		PhaseLegal:   legal,
		HandoffFresh: fresh,
		VerifyPassed: verifyPassed,
	}
	report.CanAdvance = active && has && legal && fresh
	report.CanArchive = active && c.Phase == KindTasks && legal && fresh && verifyPassed

	if !active {
		reasons = append(reasons, "change is not active")
	}
	if !has && !report.CanArchive {
		reasons = append(reasons, "change is at the final phase; archive it instead")
	}
	if !fresh {
		reasons = append(reasons, freshReason)
	}
	if !verifyPassed {
		reasons = append(reasons, verifyReason)
	}
	if len(reasons) == 0 {
		report.Reasons = []string{}
	} else {
		report.Reasons = reasons
	}
	return report, nil
}

// phaseLegal checks phase self-consistency: the change's phase and every
// phase before it must have an artifact (no illegal skip forward).
func (s *Service) phaseLegal(ctx context.Context, c *domain.Change) (bool, []string) {
	order := phaseOrder()
	idx := -1
	for i, k := range order {
		if k == c.Phase {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false, []string{"unknown phase: " + c.Phase}
	}
	latest, err := s.artifacts.ListArtifacts(ctx, c.ID)
	if err != nil {
		return false, []string{"list artifacts failed"}
	}
	present := map[string]bool{}
	for _, a := range latest {
		present[a.Kind] = true
	}
	var reasons []string
	for i := 0; i <= idx; i++ {
		if !present[order[i]] {
			reasons = append(reasons, "missing artifact: "+order[i])
		}
	}
	return len(reasons) == 0, reasons
}

// handoffFresh checks that the change entered its current phase through a
// guard-approved handoff and that no earlier-phase artifact was regenerated
// after that handoff (which would make the handoff stale). The proposal
// phase needs no entry handoff.
func (s *Service) handoffFresh(ctx context.Context, c *domain.Change) (bool, string) {
	if c.Phase == KindProposal {
		return true, ""
	}
	handoffs, err := s.changes.ListChangeHandoffs(ctx, c.ID)
	if err != nil {
		return false, "list handoffs failed"
	}
	if len(handoffs) == 0 {
		return false, "no handoff recorded for the current phase"
	}
	latest := handoffs[0]
	if latest.ToPhase != c.Phase {
		return false, "latest handoff targets phase " + latest.ToPhase + ", not " + c.Phase
	}
	order := phaseOrder()
	idx := -1
	for i, k := range order {
		if k == c.Phase {
			idx = i
			break
		}
	}
	arts, err := s.artifacts.ListArtifacts(ctx, c.ID)
	if err != nil {
		return false, "list artifacts failed"
	}
	byKind := map[string]domain.Artifact{}
	for _, a := range arts {
		byKind[a.Kind] = a
	}
	for i := 0; i < idx; i++ {
		if a, ok := byKind[order[i]]; ok && a.CreatedAt.After(latest.CreatedAt) {
			return false, "handoff is stale: " + order[i] + " was updated after it"
		}
	}
	return true, ""
}

// verifyPassed checks the latest verify report: it must exist, parse as
// YAML, and carry result: pass.
func (s *Service) verifyPassed(ctx context.Context, changeID string) (bool, string) {
	a, err := s.artifacts.GetArtifact(ctx, changeID, KindVerify, 0)
	if err == store.ErrNotFound {
		return false, "no verify report submitted"
	}
	if err != nil {
		return false, "get verify report failed"
	}
	report, err := ParseVerifyReport(a.Content)
	if err != nil {
		return false, "verify report is invalid: " + err.Error()
	}
	if report.Result != "pass" {
		return false, "verify result is " + report.Result
	}
	return true, ""
}

// AdvancePhase moves the change to the next classic phase behind the guard:
// the phase must be self-consistent and freshly handed off, and the advance
// is recorded as a handoff.
func (s *Service) AdvancePhase(ctx context.Context, userID, changeID string) (*domain.Change, *domain.ChangeHandoff, error) {
	c, err := s.requireChangeRole(ctx, userID, changeID)
	if err != nil {
		return nil, nil, err
	}
	if c.Status != "active" {
		return nil, nil, httpapi.ErrConflict("change is not active")
	}
	next, has := NextPhase(c.Phase)
	if !has {
		return nil, nil, httpapi.ErrConflict("change is at the final phase; archive it instead")
	}
	if legal, reasons := s.phaseLegal(ctx, c); !legal {
		return nil, nil, httpapi.ErrConflict("phase gate failed: " + reasons[0])
	}
	if fresh, reason := s.handoffFresh(ctx, c); !fresh {
		return nil, nil, httpapi.ErrConflict("handoff gate failed: " + reason)
	}
	handoff, err := s.changes.CreateChangeHandoff(ctx, &domain.ChangeHandoff{
		ChangeID:  c.ID,
		FromPhase: c.Phase,
		ToPhase:   next,
		CreatedBy: userID,
	})
	if err != nil {
		return nil, nil, httpapi.ErrInternal("record handoff failed")
	}
	c.Phase = next
	if _, err := s.changes.UpdateChange(ctx, c); err != nil {
		return nil, nil, httpapi.ErrInternal("advance change phase failed")
	}
	return c, handoff, nil
}

// SubmitVerifyReport stores a verify report as a versioned artifact. The
// YAML must parse and carry result pass or fail; only a pass report
// releases the archive gate.
func (s *Service) SubmitVerifyReport(ctx context.Context, userID, changeID, content string) (*domain.Artifact, bool, error) {
	c, err := s.requireChangeRole(ctx, userID, changeID)
	if err != nil {
		return nil, false, err
	}
	if c.Status != "active" {
		return nil, false, httpapi.ErrConflict("change is not active")
	}
	report, err := ParseVerifyReport(content)
	if err != nil {
		return nil, false, httpapi.ErrInvalid("verify report rejected: "+err.Error())
	}
	a, err := s.artifacts.CreateArtifact(ctx, &domain.Artifact{
		ChangeID:  c.ID,
		Kind:      KindVerify,
		Content:   content,
		CreatedBy: userID,
	})
	if err != nil {
		return nil, false, httpapi.ErrInternal("save verify report failed")
	}
	return a, report.Result == "pass", nil
}

// Archive closes out the change once every gate passes: active, at the
// tasks phase with a self-consistent artifact chain, a fresh handoff, and a
// verify report with result pass. Archiving wakes the owner of the change
// issue's parent for acceptance, mirroring the Multica rule that a parent
// is woken when its children reach terminal states.
func (s *Service) Archive(ctx context.Context, userID, changeID string) (*domain.Change, error) {
	c, err := s.requireChangeRole(ctx, userID, changeID)
	if err != nil {
		return nil, err
	}
	if c.Status != "active" {
		return nil, httpapi.ErrConflict("change is not active")
	}
	if c.Phase != KindTasks {
		return nil, httpapi.ErrConflict("archive gate failed: phase is not tasks")
	}
	if legal, reasons := s.phaseLegal(ctx, c); !legal {
		return nil, httpapi.ErrConflict("archive gate failed: " + reasons[0])
	}
	if fresh, reason := s.handoffFresh(ctx, c); !fresh {
		return nil, httpapi.ErrConflict("archive gate failed: " + reason)
	}
	if passed, reason := s.verifyPassed(ctx, c.ID); !passed {
		return nil, httpapi.ErrConflict("archive gate failed: " + reason)
	}
	if s.wakeups == nil {
		return nil, httpapi.ErrInternal("wakeup recorder is not configured")
	}
	c.Status = "archived"
	if _, err := s.changes.UpdateChange(ctx, c); err != nil {
		return nil, httpapi.ErrInternal("archive change failed")
	}
	if err := s.wakeChangeIssueParent(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// wakeChangeIssueParent records a wakeup on the change issue's parent, if
// the issue is itself a child.
func (s *Service) wakeChangeIssueParent(ctx context.Context, c *domain.Change) error {
	i, err := s.issues.GetIssue(ctx, c.IssueID)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return httpapi.ErrInternal("get issue failed")
	}
	if i.ParentID == "" {
		return nil
	}
	if err := s.wakeups.CreateIssueWakeup(ctx, i.ParentID, i.ID); err != nil {
		return httpapi.ErrInternal("record parent wakeup failed")
	}
	if s.wakeupHook != nil {
		parent, err := s.issues.GetIssue(ctx, i.ParentID)
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("parent issue not found")
		}
		if err != nil {
			return httpapi.ErrInternal("get parent issue failed")
		}
		if err := s.wakeupHook.OnParentWakeup(ctx, parent); err != nil {
			return httpapi.ErrInternal("notify parent wakeup failed")
		}
	}
	return nil
}
