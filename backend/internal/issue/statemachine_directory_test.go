package issue

import (
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

// customDirectory is a directory with workspace-defined statuses mapped to
// the fixed categories: "qa_review" behaves like in_review, "shipped" like
// done. The built-ins it drops (backlog/blocked) must stop being valid.
func customDirectory() []domain.WorkspaceStatus {
	return []domain.WorkspaceStatus{
		{Name: "todo", Category: domain.CatTodo, Position: 0},
		{Name: "doing", Category: domain.CatInProgress, Position: 1},
		{Name: "qa_review", Category: domain.CatInReview, Position: 2},
		{Name: "shipped", Category: domain.CatDone, Position: 3},
		{Name: "dropped", Category: domain.CatCancelled, Position: 4},
	}
}

func TestTransitionInDefaultDirectoryMatchesTransition(t *testing.T) {
	dir := domain.DefaultStatusDirectory()
	from, err := TransitionIn(dir, StatusTodo, StatusInProgress)
	if err != nil || from != StatusInProgress {
		t.Fatalf("TransitionIn(todo, in_progress) = %q, %v; want in_progress, nil", from, err)
	}
	if _, err := TransitionIn(dir, StatusDone, StatusTodo); err == nil {
		t.Fatal("TransitionIn(done, todo) should fail")
	}
	if _, err := TransitionIn(dir, StatusTodo, StatusTodo); err == nil {
		t.Fatal("same-status transition should fail (todo category has no self transition)")
	}
}

func TestTransitionInCustomDirectory(t *testing.T) {
	dir := customDirectory()

	t.Run("moves follow the category machine", func(t *testing.T) {
		cases := []struct {
			from, to string
			legal    bool
		}{
			{"todo", "doing", true},         // todo -> in_progress
			{"doing", "qa_review", true},    // in_progress -> in_review
			{"qa_review", "shipped", true},  // in_review -> done
			{"qa_review", "doing", true},    // rework
			{"shipped", "doing", false},     // done is terminal
			{"dropped", "todo", false},      // cancelled is terminal
			{"todo", "qa_review", false},    // todo -> in_review is illegal
			{"todo", "shipped", false},      // todo -> done is illegal
			{"qa_review", "dropped", true},  // in_review -> cancelled
			{"todo", "todo", false},         // same category, no self transition
		}
		for _, tc := range cases {
			_, err := TransitionIn(dir, tc.from, tc.to)
			if tc.legal && err != nil {
				t.Errorf("TransitionIn(%q, %q) unexpected error: %v", tc.from, tc.to, err)
			}
			if !tc.legal && err == nil {
				t.Errorf("TransitionIn(%q, %q) should be illegal", tc.from, tc.to)
			}
		}
	})

	t.Run("statuses outside the directory are rejected", func(t *testing.T) {
		for _, s := range []string{StatusBacklog, StatusBlocked, StatusInReview, StatusDone, StatusCancelled} {
			if _, err := TransitionIn(dir, StatusTodo, s); err == nil {
				appErr, ok := err.(*httpapi.AppError)
				if !ok || appErr.Status != 400 {
					t.Fatalf("TransitionIn(todo, %q) error = %v, want 400 invalid", s, err)
				}
			}
		}
		if _, err := TransitionIn(dir, "blocked", "todo"); err == nil {
			t.Error("from-status outside the directory should be rejected")
		}
	})

	t.Run("terminal state follows the mapped category", func(t *testing.T) {
		if !IsTerminalIn(dir, "shipped") || !IsTerminalIn(dir, "dropped") {
			t.Error("done/cancelled category statuses should be terminal")
		}
		if IsTerminalIn(dir, "qa_review") || IsTerminalIn(dir, "todo") {
			t.Error("non-terminal category statuses should not be terminal")
		}
		if IsTerminalIn(dir, "backlog") {
			t.Error("status outside the directory should not be terminal")
		}
	})
}
