// Package agent implements the agent runtime: agent definitions (backed by
// user rows so issues can be assigned to them), the run queue triggered by
// issue assignment and status changes, and the LLM tool-loop executor that
// reads the issue, checks out repositories, posts comments and updates
// status, logging every turn.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
)

// Service manages agent definitions.
type Service struct {
	agents store.AgentStore
	users  store.UserStore
	skills *skill.Registry
}

func NewService(agents store.AgentStore, users store.UserStore, skills *skill.Registry) *Service {
	return &Service{agents: agents, users: users, skills: skills}
}

type CreateInput struct {
	Name        string
	Description string
	Skills      []string
}

type UpdateInput struct {
	Name        *string
	Description *string
	Skills      []string
}

func (s *Service) validateSkills(ctx context.Context, skills []string) error {
	seen := map[string]bool{}
	for _, key := range skills {
		if seen[key] {
			return httpapi.ErrInvalid("duplicate skill: " + key)
		}
		seen[key] = true
		if _, ok := s.skills.Get(key); !ok {
			return httpapi.ErrInvalid("unknown skill: " + key)
		}
	}
	return nil
}

// CreateAgent provisions the agent's backing user row (generated email and
// password — agents never log in) and the agent definition.
func (s *Service) CreateAgent(ctx context.Context, creatorID string, in CreateInput) (*domain.Agent, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, httpapi.ErrInvalid("agent name is required")
	}
	if err := s.validateSkills(ctx, in.Skills); err != nil {
		return nil, err
	}
	password := make([]byte, 32)
	if _, err := rand.Read(password); err != nil {
		return nil, httpapi.ErrInternal("generate agent password failed")
	}
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, httpapi.ErrInternal("hash agent password failed")
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return nil, httpapi.ErrInternal("generate agent email failed")
	}
	email := "agent-" + hex.EncodeToString(suffix) + "@agents.local"

	u, err := s.users.CreateUser(ctx, email, string(hash), in.Name)
	if err != nil {
		return nil, httpapi.ErrInternal("create agent user failed")
	}
	a, err := s.agents.CreateAgent(ctx, &domain.Agent{
		ID:          u.ID,
		Name:        in.Name,
		Description: in.Description,
		Skills:      in.Skills,
		CreatedBy:   creatorID,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("create agent failed")
	}
	return a, nil
}

func (s *Service) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	a, err := s.agents.GetAgent(ctx, id)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("agent not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get agent failed")
	}
	return a, nil
}

func (s *Service) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	list, err := s.agents.ListAgents(ctx)
	if err != nil {
		return nil, httpapi.ErrInternal("list agents failed")
	}
	return list, nil
}

func (s *Service) UpdateAgent(ctx context.Context, id string, in UpdateInput) (*domain.Agent, error) {
	current, err := s.GetAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, httpapi.ErrInvalid("agent name is required")
		}
		current.Name = *in.Name
	}
	if in.Description != nil {
		current.Description = *in.Description
	}
	if in.Skills != nil {
		if err := s.validateSkills(ctx, in.Skills); err != nil {
			return nil, err
		}
		current.Skills = in.Skills
	}
	updated, err := s.agents.UpdateAgent(ctx, current)
	if err != nil {
		return nil, httpapi.ErrInternal("update agent failed")
	}
	return updated, nil
}

func (s *Service) DeleteAgent(ctx context.Context, id string) error {
	if _, err := s.GetAgent(ctx, id); err != nil {
		return err
	}
	if err := s.agents.DeleteAgent(ctx, id); err != nil {
		return httpapi.ErrInternal("delete agent failed")
	}
	return nil
}
