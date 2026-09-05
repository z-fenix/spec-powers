// Package squad implements squads: standing groups of members (human users
// or agents) with a single leader. Issues can be assigned to a squad; its
// leader then claims the issue or reassigns it.
package squad

import (
	"context"
	"errors"
	"strings"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type Service struct {
	squads store.SquadStore
	users  store.UserStore
}

func NewService(squads store.SquadStore, users store.UserStore) *Service {
	return &Service{squads: squads, users: users}
}

type CreateInput struct {
	Name        string
	Description string
	LeaderID    string
}

type UpdateInput struct {
	Name        *string
	Description *string
}

func (s *Service) requireUser(ctx context.Context, userID string) error {
	if userID == "" {
		return httpapi.ErrInvalid("user is required")
	}
	if _, err := s.users.GetUser(ctx, userID); errors.Is(err, store.ErrNotFound) {
		return httpapi.ErrNotFound("user not found")
	} else if err != nil {
		return httpapi.ErrInternal("lookup user failed")
	}
	return nil
}

// CreateSquad creates a squad and puts its leader on the roster.
func (s *Service) CreateSquad(ctx context.Context, creatorID string, in CreateInput) (*domain.Squad, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, httpapi.ErrInvalid("squad name is required")
	}
	if err := s.requireUser(ctx, in.LeaderID); err != nil {
		return nil, err
	}
	sq, err := s.squads.CreateSquad(ctx, &domain.Squad{
		Name:        in.Name,
		Description: in.Description,
		LeaderID:    in.LeaderID,
		CreatedBy:   creatorID,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("create squad failed")
	}
	if err := s.squads.AddSquadMember(ctx, sq.ID, in.LeaderID); err != nil {
		return nil, httpapi.ErrInternal("add squad leader to roster failed")
	}
	return sq, nil
}

// GetSquad returns the squad with its resolved roster.
func (s *Service) GetSquad(ctx context.Context, id string) (*domain.Squad, []domain.SquadMemberDetail, error) {
	sq, err := s.squads.GetSquad(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, httpapi.ErrNotFound("squad not found")
	}
	if err != nil {
		return nil, nil, httpapi.ErrInternal("get squad failed")
	}
	members, err := s.squads.ListSquadMemberDetails(ctx, id)
	if err != nil {
		return nil, nil, httpapi.ErrInternal("list squad members failed")
	}
	return sq, members, nil
}

func (s *Service) ListSquads(ctx context.Context) ([]domain.Squad, error) {
	list, err := s.squads.ListSquads(ctx)
	if err != nil {
		return nil, httpapi.ErrInternal("list squads failed")
	}
	return list, nil
}

func (s *Service) UpdateSquad(ctx context.Context, id string, in UpdateInput) (*domain.Squad, error) {
	current, err := s.squads.GetSquad(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpapi.ErrNotFound("squad not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get squad failed")
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, httpapi.ErrInvalid("squad name is required")
		}
		current.Name = *in.Name
	}
	if in.Description != nil {
		current.Description = *in.Description
	}
	updated, err := s.squads.UpdateSquad(ctx, current)
	if err != nil {
		return nil, httpapi.ErrInternal("update squad failed")
	}
	return updated, nil
}

// SetLeader hands leadership to another user; the new leader is added to the
// roster when not already a member.
func (s *Service) SetLeader(ctx context.Context, id, leaderID string) (*domain.Squad, error) {
	if _, err := s.squads.GetSquad(ctx, id); errors.Is(err, store.ErrNotFound) {
		return nil, httpapi.ErrNotFound("squad not found")
	} else if err != nil {
		return nil, httpapi.ErrInternal("get squad failed")
	}
	if err := s.requireUser(ctx, leaderID); err != nil {
		return nil, err
	}
	sq, err := s.squads.SetSquadLeader(ctx, id, leaderID)
	if err != nil {
		return nil, httpapi.ErrInternal("set squad leader failed")
	}
	members, err := s.squads.ListSquadMembers(ctx, id)
	if err != nil {
		return nil, httpapi.ErrInternal("list squad members failed")
	}
	for _, m := range members {
		if m.UserID == leaderID {
			return sq, nil
		}
	}
	if err := s.squads.AddSquadMember(ctx, id, leaderID); err != nil {
		return nil, httpapi.ErrInternal("add squad leader to roster failed")
	}
	return sq, nil
}

func (s *Service) AddMember(ctx context.Context, id, userID string) error {
	if _, err := s.squads.GetSquad(ctx, id); errors.Is(err, store.ErrNotFound) {
		return httpapi.ErrNotFound("squad not found")
	} else if err != nil {
		return httpapi.ErrInternal("get squad failed")
	}
	if err := s.requireUser(ctx, userID); err != nil {
		return err
	}
	if err := s.squads.AddSquadMember(ctx, id, userID); errors.Is(err, store.ErrConflict) {
		return httpapi.ErrConflict("user is already a squad member")
	} else if err != nil {
		return httpapi.ErrInternal("add squad member failed")
	}
	return nil
}

// RemoveMember drops a roster entry; the current leader cannot be removed
// (transfer leadership first).
func (s *Service) RemoveMember(ctx context.Context, id, userID string) error {
	sq, err := s.squads.GetSquad(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return httpapi.ErrNotFound("squad not found")
	}
	if err != nil {
		return httpapi.ErrInternal("get squad failed")
	}
	if sq.LeaderID == userID {
		return httpapi.ErrInvalid("squad leader cannot be removed")
	}
	if err := s.squads.RemoveSquadMember(ctx, id, userID); errors.Is(err, store.ErrNotFound) {
		return httpapi.ErrNotFound("member not found")
	} else if err != nil {
		return httpapi.ErrInternal("remove squad member failed")
	}
	return nil
}

func (s *Service) DeleteSquad(ctx context.Context, id string) error {
	if err := s.squads.DeleteSquad(ctx, id); errors.Is(err, store.ErrNotFound) {
		return httpapi.ErrNotFound("squad not found")
	} else if err != nil {
		return httpapi.ErrInternal("delete squad failed")
	}
	return nil
}
