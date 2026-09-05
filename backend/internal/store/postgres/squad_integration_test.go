package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// seedSquads seeds three users (creator, leader, agent-backed member) and one
// squad led by the leader. Cleanup deletes the users, cascading to squads and
// memberships.
func seedSquads(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (creatorID, leaderID, agentUserID, squadID string) {
	t.Helper()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, u := range []struct {
		email string
		id    *string
	}{
		{"squad-creator@example.com", &creatorID},
		{"squad-leader@example.com", &leaderID},
		{"squad-agent@example.com", &agentUserID},
	} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email, password_hash, display_name)
			VALUES ($1, 'x', 'user')
			RETURNING id`, u.email).Scan(u.id); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		id := *u.id
		t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id) })
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, name, created_by) VALUES ($1, 'SquadAgent', $2)`,
		agentUserID, creatorID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	sq, err := NewSquadStore(pool).CreateSquad(ctx, &domain.Squad{
		Name: "Platform", Description: "platform work", LeaderID: leaderID, CreatedBy: creatorID,
	})
	if err != nil {
		t.Fatalf("create squad: %v", err)
	}
	return creatorID, leaderID, agentUserID, sq.ID
}

func TestSquadStoreCRUD(t *testing.T) {
	dsn := os.Getenv("SP_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SP_TEST_PG_DSN not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	_, leaderID, agentUserID, squadID := seedSquads(t, ctx, pool)
	squads := NewSquadStore(pool)

	got, err := squads.GetSquad(ctx, squadID)
	if err != nil {
		t.Fatalf("get squad: %v", err)
	}
	if got.LeaderID != leaderID || got.Name != "Platform" {
		t.Errorf("squad = %+v, want leader %s and name Platform", got, leaderID)
	}

	if err := squads.AddSquadMember(ctx, squadID, agentUserID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := squads.AddSquadMember(ctx, squadID, agentUserID); !errors.Is(err, store.ErrConflict) {
		t.Errorf("duplicate add error = %v, want ErrConflict", err)
	}

	details, err := squads.ListSquadMemberDetails(ctx, squadID)
	if err != nil {
		t.Fatalf("list member details: %v", err)
	}
	if len(details) != 2 {
		t.Fatalf("member details = %d entries, want 2 (leader seeded)", len(details))
	}
	var agentSeen bool
	for _, d := range details {
		if d.UserID == agentUserID {
			agentSeen = true
			if !d.IsAgent || d.DisplayName == "" {
				t.Errorf("agent member detail = %+v, want IsAgent and display name", d)
			}
		}
	}
	if !agentSeen {
		t.Errorf("agent member missing from details: %+v", details)
	}

	updated, err := squads.SetSquadLeader(ctx, squadID, agentUserID)
	if err != nil {
		t.Fatalf("set leader: %v", err)
	}
	if updated.LeaderID != agentUserID {
		t.Errorf("leader = %s, want %s", updated.LeaderID, agentUserID)
	}

	if err := squads.RemoveSquadMember(ctx, squadID, leaderID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if err := squads.RemoveSquadMember(ctx, squadID, leaderID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("remove absent member error = %v, want ErrNotFound", err)
	}

	list, err := squads.ListSquads(ctx)
	if err != nil {
		t.Fatalf("list squads: %v", err)
	}
	var found bool
	for _, sq := range list {
		if sq.ID == squadID {
			found = true
		}
	}
	if !found {
		t.Errorf("squad %s missing from ListSquads", squadID)
	}

	if err := squads.DeleteSquad(ctx, squadID); err != nil {
		t.Fatalf("delete squad: %v", err)
	}
	if _, err := squads.GetSquad(ctx, squadID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("get deleted squad error = %v, want ErrNotFound", err)
	}
}
