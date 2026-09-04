package collab

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
)

func TestAddCommentNotifiesObserver(t *testing.T) {
	f := newFixture(t)
	var seen []*domain.IssueComment
	f.svc.WithCommentObserver(func(_ context.Context, c *domain.IssueComment) {
		seen = append(seen, c)
	})

	ctx := context.Background()
	created, err := f.svc.AddComment(ctx, "alice", "i1", "", "ping @KunCoding")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(seen))
	}
	if seen[0].ID != created.ID || seen[0].IssueID != "i1" ||
		seen[0].AuthorID != "alice" || seen[0].Content != "ping @KunCoding" {
		t.Fatalf("observed comment = %+v", seen[0])
	}

	// Replies are observed too, with the parent link intact.
	root, _ := f.svc.AddComment(ctx, "alice", "i1", "", "root")
	f.svc.AddComment(ctx, "bob", "i1", root.ID, "reply")
	if len(seen) != 3 {
		t.Fatalf("observer calls = %d, want 3", len(seen))
	}
	if seen[2].ParentID != root.ID {
		t.Fatalf("reply parent = %q, want %q", seen[2].ParentID, root.ID)
	}
}
