package agent

import (
	"context"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/notification"
	"specpowers/backend/internal/store"
)

// MentionTrigger auto-claims tasks for agents mentioned in comments: when a
// comment contains "@<agent name>", a run is enqueued for that agent on the
// comment's issue (trigger "mention"), unless the agent authored the comment
// itself (no self-loops).
type MentionTrigger struct {
	agents store.AgentStore
	runs   store.RunStore
}

func NewMentionTrigger(agents store.AgentStore, runs store.RunStore) *MentionTrigger {
	return &MentionTrigger{agents: agents, runs: runs}
}

// OnComment scans one comment's content for agent mentions and enqueues
// their runs.
func (t *MentionTrigger) OnComment(ctx context.Context, issueID, authorID, content string) error {
	list, err := t.agents.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, a := range list {
		if a.ID == authorID || !mentionsAgent(content, a.Name) {
			continue
		}
		if _, err := t.runs.CreateRun(ctx, &domain.Run{
			AgentID: a.ID,
			IssueID: issueID,
			Trigger: "mention",
		}); err != nil {
			return err
		}
	}
	return nil
}

// mentionsAgent reports whether content mentions the agent as @<name>
// (case-insensitive); the syntax is shared with human mention
// notifications.
func mentionsAgent(content, name string) bool {
	return notification.MentionsName(content, name)
}
