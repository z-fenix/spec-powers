package issue

import (
	"context"
	"testing"

	"specpowers/backend/internal/httpapi"
)

func TestSubIssueCreation(t *testing.T) {
	ctx := context.Background()

	t.Run("child groups position per (parent, stage)", func(t *testing.T) {
		svc, _, _, _ := newService()
		parent, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "parent"})
		c1, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c1", ParentID: parent.ID})
		if err != nil {
			t.Fatalf("c1: %v", err)
		}
		c2, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c2", ParentID: parent.ID, Stage: 1})
		if err != nil {
			t.Fatalf("c2: %v", err)
		}
		if c1.ParentID != parent.ID || c2.ParentID != parent.ID {
			t.Fatalf("parent not set: %+v %+v", c1, c2)
		}
		if c1.Position != 0 {
			t.Errorf("c1 position = %d, want 0", c1.Position)
		}
		if c2.Position != 0 {
			t.Errorf("c2 position = %d, want 0 (stage 1 is its own group)", c2.Position)
		}
		if c1.Stage != 0 || c2.Stage != 1 {
			t.Errorf("stages = %d/%d, want 0/1", c1.Stage, c2.Stage)
		}
	})

	t.Run("unknown parent is 404", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c", ParentID: "missing"})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})

	t.Run("parent in another project is rejected", func(t *testing.T) {
		svc, _, projects, _ := newService()
		projects.existing["p2"] = true
		projects.members["p2|alice"] = "owner"
		foreign, err := svc.CreateIssue(ctx, "alice", "p2", CreateInput{Title: "foreign"})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		_, err = svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c", ParentID: foreign.ID})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})
}

func TestParentMove(t *testing.T) {
	ctx := context.Background()

	// setup builds a -> b -> c and returns their IDs.
	setup := func(t *testing.T) (*Service, string, string, string) {
		t.Helper()
		svc, _, _, _ := newService()
		a, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		b, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "b", ParentID: a.ID})
		c, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c", ParentID: b.ID})
		return svc, a.ID, b.ID, c.ID
	}

	t.Run("self parent is rejected", func(t *testing.T) {
		svc, a, _, _ := setup(t)
		_, err := svc.UpdateIssue(ctx, "alice", a, UpdateInput{ParentID: ptrString(a)})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})

	t.Run("descendant parent is rejected (cycle)", func(t *testing.T) {
		svc, a, _, c := setup(t)
		_, err := svc.UpdateIssue(ctx, "alice", a, UpdateInput{ParentID: ptrString(c)})
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})

	t.Run("reparenting to unrelated issue succeeds", func(t *testing.T) {
		svc, _, _, c := setup(t)
		d, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "d"})
		if _, err := svc.UpdateIssue(ctx, "alice", c, UpdateInput{ParentID: ptrString(d.ID)}); err != nil {
			t.Fatalf("move c under d: %v", err)
		}
		got, _ := svc.GetIssue(ctx, "alice", c)
		if got.ParentID != d.ID {
			t.Fatalf("c parent = %q, want %q", got.ParentID, d.ID)
		}
	})

	t.Run("clearing parent makes it a root issue", func(t *testing.T) {
		svc, _, _, c := setup(t)
		if _, err := svc.UpdateIssue(ctx, "alice", c, UpdateInput{ParentID: ptrString("")}); err != nil {
			t.Fatalf("clear parent: %v", err)
		}
		got, _ := svc.GetIssue(ctx, "alice", c)
		if got.ParentID != "" {
			t.Fatalf("c parent = %q, want empty", got.ParentID)
		}
	})
}

func TestGetChildren(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newService()
	parent, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "parent"})
	// children across stages; created out of stage order on purpose.
	c2, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c2", ParentID: parent.ID, Stage: 2})
	c0b, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c0b", ParentID: parent.ID, Stage: 0})
	c0a, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c0a", ParentID: parent.ID, Stage: 0})
	// sibling reordering within stage 0: c0a moves after c0b.
	if _, err := svc.UpdateIssue(ctx, "alice", c0a.ID, UpdateInput{Position: ptrInt(2)}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// an unrelated issue that must not appear
	_, _ = svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "other"})

	t.Run("returns children ordered by stage then position", func(t *testing.T) {
		kids, err := svc.GetChildren(ctx, "bob", parent.ID)
		if err != nil {
			t.Fatalf("GetChildren: %v", err)
		}
		want := []string{c0b.ID, c0a.ID, c2.ID}
		if len(kids) != len(want) {
			t.Fatalf("len = %d, want %d", len(kids), len(want))
		}
		for k, kid := range kids {
			if kid.ID != want[k] {
				t.Fatalf("order[%d] = %s, want %s", k, kid.ID, want[k])
			}
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		_, err := svc.GetChildren(ctx, "alice", "missing")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}
