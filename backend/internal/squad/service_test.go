package squad

import (
	"context"
	"errors"
	"sort"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type fakeSquads struct {
	byID    map[string]*domain.Squad
	nextID  int
	members map[string][]string // squadID -> userIDs in join order
}

func newFakeSquads() *fakeSquads {
	return &fakeSquads{
		byID:    map[string]*domain.Squad{},
		members: map[string][]string{},
	}
}

func (f *fakeSquads) CreateSquad(_ context.Context, s *domain.Squad) (*domain.Squad, error) {
	f.nextID++
	clone := *s
	clone.ID = string(rune('S' + f.nextID))
	f.byID[clone.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeSquads) GetSquad(_ context.Context, id string) (*domain.Squad, error) {
	sq, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *sq
	return &out, nil
}

func (f *fakeSquads) ListSquads(_ context.Context) ([]domain.Squad, error) {
	var out []domain.Squad
	for _, sq := range f.byID {
		out = append(out, *sq)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeSquads) UpdateSquad(_ context.Context, s *domain.Squad) (*domain.Squad, error) {
	if _, ok := f.byID[s.ID]; !ok {
		return nil, store.ErrNotFound
	}
	clone := *s
	f.byID[s.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeSquads) SetSquadLeader(_ context.Context, squadID, leaderID string) (*domain.Squad, error) {
	sq, ok := f.byID[squadID]
	if !ok {
		return nil, store.ErrNotFound
	}
	sq.LeaderID = leaderID
	out := *sq
	return &out, nil
}

func (f *fakeSquads) DeleteSquad(_ context.Context, id string) error {
	if _, ok := f.byID[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.byID, id)
	delete(f.members, id)
	return nil
}

func (f *fakeSquads) AddSquadMember(_ context.Context, squadID, userID string) error {
	if _, ok := f.byID[squadID]; !ok {
		return store.ErrNotFound
	}
	for _, id := range f.members[squadID] {
		if id == userID {
			return store.ErrConflict
		}
	}
	f.members[squadID] = append(f.members[squadID], userID)
	return nil
}

func (f *fakeSquads) RemoveSquadMember(_ context.Context, squadID, userID string) error {
	roster := f.members[squadID]
	for i, id := range roster {
		if id == userID {
			f.members[squadID] = append(roster[:i], roster[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeSquads) ListSquadMembers(_ context.Context, squadID string) ([]domain.SquadMember, error) {
	var out []domain.SquadMember
	for _, id := range f.members[squadID] {
		out = append(out, domain.SquadMember{SquadID: squadID, UserID: id})
	}
	return out, nil
}

func (f *fakeSquads) ListSquadMemberDetails(_ context.Context, squadID string) ([]domain.SquadMemberDetail, error) {
	var out []domain.SquadMemberDetail
	for _, id := range f.members[squadID] {
		out = append(out, domain.SquadMemberDetail{UserID: id, DisplayName: "user-" + id, IsAgent: false})
	}
	return out, nil
}

type fakeUsers struct {
	ids map[string]bool
}

func (f *fakeUsers) CreateUser(_ context.Context, _, _, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeUsers) GetUserByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, store.ErrNotFound
}

func (f *fakeUsers) GetUser(_ context.Context, id string) (*domain.User, error) {
	if !f.ids[id] {
		return nil, store.ErrNotFound
	}
	return &domain.User{ID: id}, nil
}

func newService() (*Service, *fakeSquads) {
	squads := newFakeSquads()
	users := &fakeUsers{ids: map[string]bool{"alice": true, "bob": true, "agent-1": true}}
	return NewService(squads, users), squads
}

func isInvalid(err error) bool {
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Status == 400
}

func isNotFound(err error) bool {
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Status == 404
}

func TestCreateSquadAddsLeaderToRoster(t *testing.T) {
	svc, squads := newService()
	sq, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "alice"})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	if sq.LeaderID != "alice" {
		t.Errorf("leader = %s, want alice", sq.LeaderID)
	}
	roster := squads.members[sq.ID]
	if len(roster) != 1 || roster[0] != "alice" {
		t.Errorf("roster = %v, want [alice]", roster)
	}
}

func TestCreateSquadValidation(t *testing.T) {
	svc, _ := newService()
	if _, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "  ", LeaderID: "alice"}); !isInvalid(err) {
		t.Errorf("blank name error = %v, want invalid", err)
	}
	if _, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: ""}); !isInvalid(err) {
		t.Errorf("missing leader error = %v, want invalid", err)
	}
	if _, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "ghost"}); !isNotFound(err) {
		t.Errorf("unknown leader error = %v, want not found", err)
	}
}

func TestRemoveLeaderForbidden(t *testing.T) {
	svc, _ := newService()
	sq, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "alice"})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	if err := svc.AddMember(context.Background(), sq.ID, "bob"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := svc.RemoveMember(context.Background(), sq.ID, "alice"); !isInvalid(err) {
		t.Errorf("remove leader error = %v, want invalid", err)
	}
	if err := svc.RemoveMember(context.Background(), sq.ID, "bob"); err != nil {
		t.Errorf("remove member: %v", err)
	}
}

func TestSetLeaderJoinsRoster(t *testing.T) {
	svc, squads := newService()
	sq, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "alice"})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	updated, err := svc.SetLeader(context.Background(), sq.ID, "bob")
	if err != nil {
		t.Fatalf("set leader: %v", err)
	}
	if updated.LeaderID != "bob" {
		t.Errorf("leader = %s, want bob", updated.LeaderID)
	}
	roster := squads.members[sq.ID]
	has := func(id string) bool {
		for _, m := range roster {
			if m == id {
				return true
			}
		}
		return false
	}
	if !has("bob") {
		t.Errorf("roster = %v, want bob added", roster)
	}
	if !has("alice") {
		t.Errorf("roster = %v, want alice kept as member", roster)
	}
	// setting the same leader again stays idempotent
	if _, err := svc.SetLeader(context.Background(), sq.ID, "bob"); err != nil {
		t.Errorf("re-set leader: %v", err)
	}
}

func TestAddMemberDuplicateConflicts(t *testing.T) {
	svc, _ := newService()
	sq, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "alice"})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	if err := svc.AddMember(context.Background(), sq.ID, "bob"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	err = svc.AddMember(context.Background(), sq.ID, "bob")
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 {
		t.Errorf("duplicate add error = %v, want 409 conflict", err)
	}
	if err := svc.AddMember(context.Background(), sq.ID, "ghost"); !isNotFound(err) {
		t.Errorf("unknown member error = %v, want not found", err)
	}
}

func TestGetSquadReturnsRoster(t *testing.T) {
	svc, _ := newService()
	sq, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "alice"})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	if err := svc.AddMember(context.Background(), sq.ID, "agent-1"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	_, members, err := svc.GetSquad(context.Background(), sq.ID)
	if err != nil {
		t.Fatalf("get squad: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	if _, _, err := svc.GetSquad(context.Background(), "nope"); !isNotFound(err) {
		t.Errorf("missing squad error = %v, want not found", err)
	}
}

func TestUpdateAndDeleteSquad(t *testing.T) {
	svc, squads := newService()
	sq, err := svc.CreateSquad(context.Background(), "creator", CreateInput{Name: "Platform", LeaderID: "alice"})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	name := "Infra"
	desc := "infra work"
	updated, err := svc.UpdateSquad(context.Background(), sq.ID, UpdateInput{Name: &name, Description: &desc})
	if err != nil {
		t.Fatalf("update squad: %v", err)
	}
	if updated.Name != name || updated.Description != desc {
		t.Errorf("updated squad = %+v", updated)
	}
	blank := " "
	if _, err := svc.UpdateSquad(context.Background(), sq.ID, UpdateInput{Name: &blank}); !isInvalid(err) {
		t.Errorf("blank rename error = %v, want invalid", err)
	}
	if err := svc.DeleteSquad(context.Background(), sq.ID); err != nil {
		t.Fatalf("delete squad: %v", err)
	}
	if _, ok := squads.byID[sq.ID]; ok {
		t.Errorf("squad still present after delete")
	}
	if err := svc.DeleteSquad(context.Background(), sq.ID); !isNotFound(err) {
		t.Errorf("delete missing squad error = %v, want not found", err)
	}
}
