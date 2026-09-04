package workflow

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

type fakeAgentAccess struct {
	agents map[string]bool
}

func (f *fakeAgentAccess) IsAgent(_ context.Context, userID string) bool {
	return f.agents[userID]
}

// Agent runs are system-driven: an agent identity must be able to act on the
// change of its assigned issue without being a project member, while humans
// keep the membership requirement.
func TestAgentAccessBypassesProjectMembership(t *testing.T) {
	f := newFixture()
	f.changes.byIssue["i1"] = &domain.Change{ID: "c1", ProjectID: "p1", IssueID: "i1", Phase: "proposal", Status: "active"}
	f.svc = f.svc.WithAgentAccess(&fakeAgentAccess{agents: map[string]bool{"agent-1": true}})

	t.Run("agent acts without membership", func(t *testing.T) {
		c, err := f.svc.GetChangeByIssue(context.Background(), "agent-1", "i1")
		if err != nil {
			t.Fatalf("agent get change: %v", err)
		}
		if c.ID != "c1" {
			t.Fatalf("change = %+v", c)
		}
	})

	t.Run("human non-member still forbidden", func(t *testing.T) {
		_, err := f.svc.GetChangeByIssue(context.Background(), "eve", "i1")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("error = %v, want 403", err)
		}
	})

	t.Run("human member unaffected", func(t *testing.T) {
		if _, err := f.svc.GetChangeByIssue(context.Background(), "bob", "i1"); err != nil {
			t.Fatalf("member get change: %v", err)
		}
	})
}
