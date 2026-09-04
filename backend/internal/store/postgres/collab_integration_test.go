package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
)

// seedCollabIssue creates a user, workspace, project and issue to hang
// collaboration data off, returning the issue and author IDs.
func seedCollabIssue(t *testing.T, pool *pgxpool.Pool) (*domain.Issue, string) {
	t.Helper()
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)
	issues := NewIssueStore(pool)

	author, err := users.CreateUser(ctx, "collab-author@example.com", "h", "Author")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS-collab", author.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	p, err := projects.CreateProject(ctx, ws.ID, "Collab", "desc", author.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	i, err := issues.CreateIssue(ctx, &domain.Issue{
		ProjectID: p.ID, Title: "collab target", Status: "todo", Priority: "none", CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	return i, author.ID
}

func TestCommentStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	issue, author := seedCollabIssue(t, pool)
	ctx := context.Background()
	comments := NewCommentStore(pool)

	root, err := comments.CreateComment(ctx, &domain.IssueComment{
		IssueID: issue.ID, ParentID: "", AuthorID: author, Content: "root",
	})
	if err != nil {
		t.Fatalf("create root comment: %v", err)
	}
	if root.ID == "" || root.IssueID != issue.ID || root.ParentID != "" {
		t.Errorf("root comment = %+v", root)
	}

	reply, err := comments.CreateComment(ctx, &domain.IssueComment{
		IssueID: issue.ID, ParentID: root.ID, AuthorID: author, Content: "reply",
	})
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if reply.ParentID != root.ID {
		t.Errorf("reply ParentID = %q, want %q", reply.ParentID, root.ID)
	}

	list, err := comments.ListComments(ctx, issue.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list[0].ID != root.ID || list[1].ID != reply.ID {
		t.Errorf("list order = %s,%s; want root first", list[0].ID, list[1].ID)
	}

	got, err := comments.GetComment(ctx, root.ID)
	if err != nil || got.Content != "root" {
		t.Errorf("get = %+v, %v", got, err)
	}
	if _, err := comments.GetComment(ctx, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Errorf("missing comment error = %v, want ErrNotFound", err)
	}

	// comments cascade away with their issue
	if err := NewIssueStore(pool).DeleteIssue(ctx, issue.ID); err != nil {
		t.Fatalf("delete issue: %v", err)
	}
	list, err = comments.ListComments(ctx, issue.ID)
	if err != nil || len(list) != 0 {
		t.Errorf("comments survived issue delete: %+v, %v", list, err)
	}
}

func TestAttachmentStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	issue, author := seedCollabIssue(t, pool)
	ctx := context.Background()
	attachments := NewAttachmentStore(pool)
	comments := NewCommentStore(pool)

	root, _ := comments.CreateComment(ctx, &domain.IssueComment{
		IssueID: issue.ID, AuthorID: author, Content: "with file",
	})
	a, err := attachments.CreateAttachment(ctx, &domain.IssueAttachment{
		IssueID: issue.ID, CommentID: root.ID, FileName: "notes.txt",
		SizeBytes: 5, ContentType: "text/plain", StoragePath: issue.ID + "/x", UploadedBy: author,
	})
	if err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	if a.ID == "" || a.FileName != "notes.txt" || a.SizeBytes != 5 {
		t.Errorf("attachment = %+v", a)
	}

	list, err := attachments.ListAttachments(ctx, issue.ID)
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Errorf("list = %+v, %v", list, err)
	}

	got, err := attachments.GetAttachment(ctx, a.ID)
	if err != nil || got.StoragePath != a.StoragePath {
		t.Errorf("get = %+v, %v", got, err)
	}
	if _, err := attachments.GetAttachment(ctx, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Errorf("missing attachment error = %v, want ErrNotFound", err)
	}

	if err := NewIssueStore(pool).DeleteIssue(ctx, issue.ID); err != nil {
		t.Fatalf("delete issue: %v", err)
	}
	list, err = attachments.ListAttachments(ctx, issue.ID)
	if err != nil || len(list) != 0 {
		t.Errorf("attachments survived issue delete: %+v, %v", list, err)
	}
}

func TestMetadataStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	issue, _ := seedCollabIssue(t, pool)
	ctx := context.Background()
	metadata := NewIssueMetadataStore(pool)

	first, err := metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issue.ID, Key: "env", Value: "staging", Type: "string",
	})
	if err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	if first.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set on insert")
	}

	time.Sleep(10 * time.Millisecond)
	second, err := metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issue.ID, Key: "env", Value: "prod", Type: "string",
	})
	if err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}
	if second.Value != "prod" {
		t.Errorf("upsert did not overwrite: %+v", second)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("updated_at not bumped: %v -> %v", first.UpdatedAt, second.UpdatedAt)
	}

	if _, err := metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issue.ID, Key: "est", Value: "3.5", Type: "number",
	}); err != nil {
		t.Fatalf("set number: %v", err)
	}
	if _, err := metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issue.ID, Key: "flag", Value: "true", Type: "bool",
	}); err != nil {
		t.Fatalf("set bool: %v", err)
	}

	list, err := metadata.ListIssueMetadata(ctx, issue.ID)
	if err != nil || len(list) != 3 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list[0].Key != "env" || list[1].Key != "est" || list[2].Key != "flag" {
		t.Errorf("list order = %s,%s,%s; want env,est,flag", list[0].Key, list[1].Key, list[2].Key)
	}

	if _, err := metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issue.ID, Key: "bad", Value: "v", Type: "bogus",
	}); err == nil {
		t.Error("unknown type accepted")
	}
	if _, err := metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issue.ID, Key: "  ", Value: "v", Type: "string",
	}); err == nil {
		t.Error("blank key accepted")
	}

	if err := metadata.DeleteIssueMetadata(ctx, issue.ID, "env"); err != nil {
		t.Fatalf("delete metadata: %v", err)
	}
	if err := metadata.DeleteIssueMetadata(ctx, issue.ID, "env"); err != ErrNotFound {
		t.Errorf("delete missing error = %v, want ErrNotFound", err)
	}
}
