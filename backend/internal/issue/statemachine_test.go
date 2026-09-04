package issue

import (
	"testing"

	"specpowers/backend/internal/httpapi"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		// backlog is the parking state: it may enter the work stream or be dropped.
		{"backlog to todo", StatusBacklog, StatusTodo, true},
		{"backlog to cancelled", StatusBacklog, StatusCancelled, true},
		{"backlog to in_progress is illegal", StatusBacklog, StatusInProgress, false},
		// todo is the ready state.
		{"todo to in_progress", StatusTodo, StatusInProgress, true},
		{"todo to blocked", StatusTodo, StatusBlocked, true},
		{"todo to cancelled", StatusTodo, StatusCancelled, true},
		{"todo to in_review is illegal", StatusTodo, StatusInReview, false},
		// in_progress may pause, block, submit for review or be dropped.
		{"in_progress to in_review", StatusInProgress, StatusInReview, true},
		{"in_progress to blocked", StatusInProgress, StatusBlocked, true},
		{"in_progress to todo", StatusInProgress, StatusTodo, true},
		{"in_progress to cancelled", StatusInProgress, StatusCancelled, true},
		{"in_progress to done is illegal", StatusInProgress, StatusDone, false},
		// in_review is the acceptance gate.
		{"in_review to done", StatusInReview, StatusDone, true},
		{"in_review to in_progress (rework)", StatusInReview, StatusInProgress, true},
		{"in_review to cancelled", StatusInReview, StatusCancelled, true},
		{"in_review to blocked is illegal", StatusInReview, StatusBlocked, false},
		// blocked may resume into work or be dropped.
		{"blocked to in_progress", StatusBlocked, StatusInProgress, true},
		{"blocked to todo", StatusBlocked, StatusTodo, true},
		{"blocked to cancelled", StatusBlocked, StatusCancelled, true},
		{"blocked to in_review is illegal", StatusBlocked, StatusInReview, false},
		// done and cancelled are terminal.
		{"done is terminal", StatusDone, StatusInProgress, false},
		{"done to done is illegal", StatusDone, StatusDone, false},
		{"cancelled is terminal", StatusCancelled, StatusTodo, false},
		{"cancelled to cancelled is illegal", StatusCancelled, StatusCancelled, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTransition(t *testing.T) {
	t.Run("legal transition returns target status", func(t *testing.T) {
		to, err := Transition(StatusTodo, StatusInProgress)
		if err != nil {
			t.Fatalf("Transition(todo, in_progress) returned error: %v", err)
		}
		if to != StatusInProgress {
			t.Fatalf("Transition(todo, in_progress) = %q, want in_progress", to)
		}
	})

	t.Run("illegal transition returns invalid request error", func(t *testing.T) {
		_, err := Transition(StatusDone, StatusTodo)
		if err == nil {
			t.Fatal("Transition(done, todo) should fail")
		}
		appErr, ok := err.(*httpapi.AppError)
		if !ok {
			t.Fatalf("error is %T, want *httpapi.AppError", err)
		}
		if appErr.Status != 400 {
			t.Fatalf("status = %d, want 400", appErr.Status)
		}
	})

	t.Run("unknown status is rejected", func(t *testing.T) {
		if _, err := Transition("shipped", StatusTodo); err == nil {
			t.Fatal("Transition from unknown status should fail")
		}
		if _, err := Transition(StatusTodo, "shipped"); err == nil {
			t.Fatal("Transition to unknown status should fail")
		}
	})
}

func TestIsTerminal(t *testing.T) {
	for _, s := range []string{StatusDone, StatusCancelled} {
		if !IsTerminal(s) {
			t.Errorf("IsTerminal(%q) = false, want true", s)
		}
	}
	for _, s := range []string{StatusTodo, StatusInProgress, StatusInReview, StatusBlocked, StatusBacklog} {
		if IsTerminal(s) {
			t.Errorf("IsTerminal(%q) = true, want false", s)
		}
	}
}

func TestIsValidStatus(t *testing.T) {
	if IsValidStatus("shipped") {
		t.Error("IsValidStatus(shipped) = true, want false")
	}
	if !IsValidStatus(StatusBacklog) {
		t.Error("IsValidStatus(backlog) = false, want true")
	}
}
