package postgres

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// TestWorkspaceStatusStoreIntegration exercises the WorkspaceStatusStore
// against real Postgres, including the defaults/materialize semantics.
func TestWorkspaceStatusStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	var userID, workspaceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ('status-store-test@example.com', 'x', 'Status Tester')
		RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})
	if err := pool.QueryRow(ctx, `
		INSERT INTO workspaces (name, created_by) VALUES ('status-store-test', $1) RETURNING id`, userID).Scan(&workspaceID); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	s := NewWorkspaceStatusStore(pool)

	t.Run("untouched workspace yields built-in defaults", func(t *testing.T) {
		list, err := s.ListStatuses(ctx, workspaceID)
		if err != nil {
			t.Fatalf("ListStatuses: %v", err)
		}
		if len(list) != 7 {
			t.Fatalf("len = %d, want 7 defaults", len(list))
		}
		for i, st := range list {
			if st.Name != st.Category || st.Position != i {
				t.Errorf("entry %d = %+v", i, st)
			}
		}
	})

	t.Run("upsert materializes defaults and appends", func(t *testing.T) {
		created, err := s.UpsertStatus(ctx, &domain.WorkspaceStatus{
			WorkspaceID: workspaceID, Name: "qa_review", Category: domain.CatInReview, Position: 7,
		})
		if err != nil {
			t.Fatalf("UpsertStatus: %v", err)
		}
		if created.Name != "qa_review" || created.Category != domain.CatInReview {
			t.Errorf("created = %+v", created)
		}
		list, err := s.ListStatuses(ctx, workspaceID)
		if err != nil {
			t.Fatalf("ListStatuses: %v", err)
		}
		if len(list) != 8 {
			t.Fatalf("len = %d, want 8 (defaults materialized + custom)", len(list))
		}
	})

	t.Run("upsert updates an existing entry", func(t *testing.T) {
		if _, err := s.UpsertStatus(ctx, &domain.WorkspaceStatus{
			WorkspaceID: workspaceID, Name: "qa_review", Category: domain.CatDone, Position: 3,
		}); err != nil {
			t.Fatalf("UpsertStatus: %v", err)
		}
		list, _ := s.ListStatuses(ctx, workspaceID)
		if len(list) != 8 {
			t.Fatalf("len = %d, want 8", len(list))
		}
		for _, st := range list {
			if st.Name == "qa_review" && (st.Category != domain.CatDone || st.Position != 3) {
				t.Errorf("qa_review = %+v, want category done position 3", st)
			}
		}
	})

	t.Run("delete removes and reports unknown", func(t *testing.T) {
		if err := s.DeleteStatus(ctx, workspaceID, "blocked"); err != nil {
			t.Fatalf("DeleteStatus: %v", err)
		}
		list, _ := s.ListStatuses(ctx, workspaceID)
		if len(list) != 7 {
			t.Fatalf("len = %d, want 7 after delete", len(list))
		}
		if err := s.DeleteStatus(ctx, workspaceID, "ghost"); err != store.ErrNotFound {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}
