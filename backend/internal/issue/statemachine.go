package issue

import "specpowers/backend/internal/httpapi"

// Issue status values. done and cancelled are terminal.
const (
	StatusBacklog    = "backlog"
	StatusTodo       = "todo"
	StatusInProgress = "in_progress"
	StatusInReview   = "in_review"
	StatusDone       = "done"
	StatusBlocked    = "blocked"
	StatusCancelled  = "cancelled"
)

// Priority values.
const (
	PriorityNone   = "none"
	PriorityLow    = "low"
	PriorityMedium = "medium"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

var validStatuses = map[string]bool{
	StatusBacklog: true, StatusTodo: true, StatusInProgress: true,
	StatusInReview: true, StatusDone: true, StatusBlocked: true,
	StatusCancelled: true,
}

var terminalStatuses = map[string]bool{
	StatusDone: true, StatusCancelled: true,
}

// transitions is the legal state machine. Anything not listed is illegal,
// including self-transitions on terminal states.
var transitions = map[string]map[string]bool{
	StatusBacklog:    {StatusTodo: true, StatusCancelled: true},
	StatusTodo:       {StatusInProgress: true, StatusBlocked: true, StatusCancelled: true},
	StatusInProgress: {StatusInReview: true, StatusBlocked: true, StatusTodo: true, StatusCancelled: true},
	StatusInReview:   {StatusDone: true, StatusInProgress: true, StatusCancelled: true},
	StatusBlocked:    {StatusInProgress: true, StatusTodo: true, StatusCancelled: true},
	StatusDone:       {},
	StatusCancelled:  {},
}

func IsValidStatus(s string) bool { return validStatuses[s] }

func IsTerminal(s string) bool { return terminalStatuses[s] }

func CanTransition(from, to string) bool {
	return validStatuses[from] && transitions[from][to]
}

// Transition validates a status change and returns the target status.
func Transition(from, to string) (string, error) {
	if !IsValidStatus(from) {
		return "", httpapi.ErrInvalid("unknown status: " + from)
	}
	if !IsValidStatus(to) {
		return "", httpapi.ErrInvalid("unknown status: " + to)
	}
	if !transitions[from][to] {
		return "", httpapi.ErrInvalid("illegal status transition: " + from + " -> " + to)
	}
	return to, nil
}

func IsValidPriority(p string) bool {
	switch p {
	case PriorityNone, PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}
