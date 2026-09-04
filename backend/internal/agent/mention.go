package agent

import (
	"context"
	"strings"

	"specpowers/backend/internal/domain"
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
// (case-insensitive).
func mentionsAgent(content, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	idx := 0
	for {
		at := strings.Index(content[idx:], "@")
		if at < 0 {
			return false
		}
		at += idx
		rest := content[at+1:]
		if len(rest) >= len(name) && strings.EqualFold(rest[:len(name)], name) {
			// The character after the name must not extend it into a
			// different word (e.g. @KunCodingX must not match KunCoding).
			after := rest[len(name):]
			if after == "" || !isNameByte(after[0]) {
				return true
			}
		}
		idx = at + 1
	}
}

func isNameByte(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		b >= 0x80 // treat multibyte (e.g. CJK names) as part of the token
}
