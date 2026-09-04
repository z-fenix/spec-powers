package workflow

import (
	"context"

	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/skill"
)

// WithSkills attaches the skill registry used by NextSkill.
func (s *Service) WithSkills(reg *skill.Registry) *Service {
	s.skills = reg
	return s
}

// WithAgentAccess installs the agent identity lookup: agent users then act
// on changes without project membership (their runs are system-driven).
func (s *Service) WithAgentAccess(lookup agentAccessLookup) *Service {
	s.agentAccess = lookup
	return s
}

// NextSkill resolves the skill the agent should load next for the change,
// derived from the change's phase and status. A change that is not active
// (archived, failed) has no next skill.
func (s *Service) NextSkill(ctx context.Context, userID, changeID string) (*skill.Skill, error) {
	c, err := s.requireChangeRole(ctx, userID, changeID)
	if err != nil {
		return nil, err
	}
	if s.skills == nil {
		return nil, httpapi.ErrInternal("skill registry is not configured")
	}
	key := skill.NextForChange(c.Phase, c.Status)
	if key == "" {
		return nil, httpapi.ErrNotFound("no next skill for this change")
	}
	sk, ok := s.skills.Get(key)
	if !ok {
		return nil, httpapi.ErrInternal("skill " + key + " is missing from the registry")
	}
	return sk, nil
}
