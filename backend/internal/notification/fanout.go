package notification

import "context"

// SinkFunc adapts a function to the Sink interface, mainly for tests.
type SinkFunc func(ctx context.Context, in NotifyInput)

func (f SinkFunc) Notify(ctx context.Context, in NotifyInput) { f(ctx, in) }

// NotifyMany fans one event out to a set of users. The actor (skip), empty
// ids and duplicates are dropped; delivery is best-effort through Notify.
func NotifyMany(ctx context.Context, sink Sink, userIDs []string, skip string, in NotifyInput) {
	if sink == nil {
		return
	}
	seen := map[string]bool{skip: true}
	for _, id := range userIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		in.UserID = id
		sink.Notify(ctx, in)
	}
}
