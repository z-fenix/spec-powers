package notification

import (
	"context"
	"testing"
)

func TestNotifyMany(t *testing.T) {
	var got []string
	sink := SinkFunc(func(_ context.Context, in NotifyInput) {
		got = append(got, in.UserID)
	})

	t.Run("fans out to each user with the actor skipped", func(t *testing.T) {
		got = nil
		NotifyMany(context.Background(), sink, []string{"alice", "bob", "carol"}, "bob", NotifyInput{Kind: "comment"})
		if len(got) != 2 || got[0] != "alice" || got[1] != "carol" {
			t.Fatalf("recipients = %v, want [alice carol]", got)
		}
	})

	t.Run("drops duplicate recipients", func(t *testing.T) {
		got = nil
		NotifyMany(context.Background(), sink, []string{"alice", "bob", "alice"}, "", NotifyInput{})
		if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
			t.Fatalf("recipients = %v, want [alice bob]", got)
		}
	})

	t.Run("skips empty user ids", func(t *testing.T) {
		got = nil
		NotifyMany(context.Background(), sink, []string{"", "alice"}, "", NotifyInput{})
		if len(got) != 1 || got[0] != "alice" {
			t.Fatalf("recipients = %v, want [alice]", got)
		}
	})

	t.Run("nil sink is a no-op", func(t *testing.T) {
		NotifyMany(context.Background(), nil, []string{"alice"}, "", NotifyInput{})
	})
}
