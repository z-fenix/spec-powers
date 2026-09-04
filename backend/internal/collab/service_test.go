package collab

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// ---- fakes ----

type fakeIssues struct {
	byID map[string]*domain.Issue
}

func (f *fakeIssues) GetIssue(_ context.Context, id string) (*domain.Issue, error) {
	i, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *i
	return &out, nil
}

type fakeProjects struct {
	existing map[string]bool
	members  map[string]string // "project|user" -> role
}

func (f *fakeProjects) GetProject(_ context.Context, id string) (*domain.Project, error) {
	if !f.existing[id] {
		return nil, store.ErrNotFound
	}
	return &domain.Project{ID: id}, nil
}

func (f *fakeProjects) GetProjectMember(_ context.Context, projectID, userID string) (*domain.ProjectMember, error) {
	role, ok := f.members[projectID+"|"+userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.ProjectMember{ProjectID: projectID, UserID: userID, Role: role}, nil
}

type fakeComments struct {
	byID   map[string]*domain.IssueComment
	nextID int
}

func (f *fakeComments) CreateComment(_ context.Context, c *domain.IssueComment) (*domain.IssueComment, error) {
	f.nextID++
	clone := *c
	clone.ID = string(rune('C' + f.nextID))
	clone.CreatedAt = time.Date(2026, 1, 1, 0, 0, f.nextID, 0, time.UTC)
	f.byID[clone.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeComments) GetComment(_ context.Context, id string) (*domain.IssueComment, error) {
	c, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *c
	return &out, nil
}

func (f *fakeComments) ListComments(_ context.Context, issueID string) ([]domain.IssueComment, error) {
	var out []domain.IssueComment
	for _, c := range f.byID {
		if c.IssueID == issueID {
			out = append(out, *c)
		}
	}
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && out[b].CreatedAt.Before(out[b-1].CreatedAt); b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out, nil
}

type fakeAttachments struct {
	byID   map[string]*domain.IssueAttachment
	nextID int
}

func (f *fakeAttachments) CreateAttachment(_ context.Context, a *domain.IssueAttachment) (*domain.IssueAttachment, error) {
	f.nextID++
	clone := *a
	clone.ID = string(rune('A' + f.nextID))
	clone.CreatedAt = time.Date(2026, 1, 2, 0, 0, f.nextID, 0, time.UTC)
	f.byID[clone.ID] = &clone
	out := clone
	return &out, nil
}

func (f *fakeAttachments) GetAttachment(_ context.Context, id string) (*domain.IssueAttachment, error) {
	a, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *a
	return &out, nil
}

func (f *fakeAttachments) ListAttachments(_ context.Context, issueID string) ([]domain.IssueAttachment, error) {
	var out []domain.IssueAttachment
	for _, a := range f.byID {
		if a.IssueID == issueID {
			out = append(out, *a)
		}
	}
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && out[b].CreatedAt.Before(out[b-1].CreatedAt); b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out, nil
}

type fakeMetadata struct {
	entries map[string]domain.IssueMetadata // "issueID|key" -> entry
}

func (f *fakeMetadata) SetIssueMetadata(_ context.Context, m *domain.IssueMetadata) (*domain.IssueMetadata, error) {
	key := m.IssueID + "|" + m.Key
	prev, existed := f.entries[key]
	clone := *m
	if existed {
		clone.UpdatedAt = prev.UpdatedAt.Add(time.Second)
	} else {
		clone.UpdatedAt = time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	}
	f.entries[key] = clone
	out := clone
	return &out, nil
}

func (f *fakeMetadata) ListIssueMetadata(_ context.Context, issueID string) ([]domain.IssueMetadata, error) {
	var out []domain.IssueMetadata
	for k, m := range f.entries {
		if strings.HasPrefix(k, issueID+"|") {
			out = append(out, m)
		}
	}
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && out[b].Key < out[b-1].Key; b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out, nil
}

func (f *fakeMetadata) DeleteIssueMetadata(_ context.Context, issueID, key string) error {
	k := issueID + "|" + key
	if _, ok := f.entries[k]; !ok {
		return store.ErrNotFound
	}
	delete(f.entries, k)
	return nil
}

// ---- fixture ----

type fixture struct {
	svc         *Service
	comments    *fakeComments
	attachments *fakeAttachments
	metadata    *fakeMetadata
	dir         string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	issues := &fakeIssues{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", ProjectID: "p1"},
	}}
	projects := &fakeProjects{
		existing: map[string]bool{"p1": true},
		members:  map[string]string{"p1|alice": "owner", "p1|bob": "member"},
	}
	comments := &fakeComments{byID: map[string]*domain.IssueComment{}}
	attachments := &fakeAttachments{byID: map[string]*domain.IssueAttachment{}}
	metadata := &fakeMetadata{entries: map[string]domain.IssueMetadata{}}
	svc := NewService(issues, projects, comments, attachments, metadata, dir)
	return &fixture{svc: svc, comments: comments, attachments: attachments, metadata: metadata, dir: dir}
}

func requireStatus(t *testing.T, err error, want int) {
	t.Helper()
	appErr, ok := err.(*httpapi.AppError)
	if !ok || appErr.Status != want {
		t.Fatalf("err = %v, want status %d", err, want)
	}
}

// ---- comments ----

func TestAddComment(t *testing.T) {
	ctx := context.Background()

	t.Run("creates a root comment", func(t *testing.T) {
		f := newFixture(t)
		c, err := f.svc.AddComment(ctx, "alice", "i1", "", "first!")
		if err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if c.IssueID != "i1" || c.ParentID != "" || c.AuthorID != "alice" || c.Content != "first!" {
			t.Errorf("comment = %+v", c)
		}
	})

	t.Run("creates a reply to a root comment", func(t *testing.T) {
		f := newFixture(t)
		root, err := f.svc.AddComment(ctx, "alice", "i1", "", "root")
		if err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		reply, err := f.svc.AddComment(ctx, "bob", "i1", root.ID, "re: root")
		if err != nil {
			t.Fatalf("AddComment reply: %v", err)
		}
		if reply.ParentID != root.ID {
			t.Errorf("ParentID = %q, want %q", reply.ParentID, root.ID)
		}
	})

	t.Run("rejects blank content", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddComment(ctx, "alice", "i1", "", "   ")
		requireStatus(t, err, 400)
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddComment(ctx, "alice", "missing", "", "x")
		requireStatus(t, err, 404)
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddComment(ctx, "mallory", "i1", "", "x")
		requireStatus(t, err, 403)
	})

	t.Run("unknown parent comment is 404", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddComment(ctx, "alice", "i1", "ghost", "x")
		requireStatus(t, err, 404)
	})

	t.Run("parent from another issue is invalid", func(t *testing.T) {
		f := newFixture(t)
		f.comments.byID["C-other"] = &domain.IssueComment{ID: "C-other", IssueID: "i2"}
		_, err := f.svc.AddComment(ctx, "alice", "i1", "C-other", "x")
		requireStatus(t, err, 400)
	})

	t.Run("replying to a reply is invalid (single-level threads)", func(t *testing.T) {
		f := newFixture(t)
		root, _ := f.svc.AddComment(ctx, "alice", "i1", "", "root")
		reply, err := f.svc.AddComment(ctx, "bob", "i1", root.ID, "re")
		if err != nil {
			t.Fatalf("AddComment reply: %v", err)
		}
		_, err = f.svc.AddComment(ctx, "alice", "i1", reply.ID, "re: re")
		requireStatus(t, err, 400)
	})
}

func TestListComments(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the issue's comments ordered by time", func(t *testing.T) {
		f := newFixture(t)
		first, _ := f.svc.AddComment(ctx, "alice", "i1", "", "1")
		second, _ := f.svc.AddComment(ctx, "bob", "i1", "", "2")
		f.svc.AddComment(ctx, "alice", "i1", first.ID, "re")
		// a comment on another issue must not leak in
		f.comments.byID["C-noise"] = &domain.IssueComment{ID: "C-noise", IssueID: "i2", Content: "noise"}

		list, err := f.svc.ListComments(ctx, "bob", "i1")
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("len = %d, want 3", len(list))
		}
		if list[0].ID != first.ID || list[1].ID != second.ID {
			t.Errorf("order = %s,%s; want %s,%s", list[0].ID, list[1].ID, first.ID, second.ID)
		}
	})

	t.Run("requires membership and existing issue", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.ListComments(ctx, "mallory", "i1")
		requireStatus(t, err, 403)
		_, err = f.svc.ListComments(ctx, "alice", "missing")
		requireStatus(t, err, 404)
	})
}

// ---- attachments ----

func TestAddAttachment(t *testing.T) {
	ctx := context.Background()

	t.Run("stores file content and metadata", func(t *testing.T) {
		f := newFixture(t)
		a, err := f.svc.AddAttachment(ctx, "alice", "i1", "", "notes.txt", "text/plain", strings.NewReader("hello"))
		if err != nil {
			t.Fatalf("AddAttachment: %v", err)
		}
		if a.FileName != "notes.txt" || a.SizeBytes != 5 || a.ContentType != "text/plain" || a.UploadedBy != "alice" {
			t.Errorf("attachment = %+v", a)
		}
		if a.CommentID != "" || a.IssueID != "i1" {
			t.Errorf("association wrong: %+v", a)
		}
		data, err := os.ReadFile(filepath.Join(f.dir, a.StoragePath))
		if err != nil {
			t.Fatalf("stored file missing: %v", err)
		}
		if string(data) != "hello" {
			t.Errorf("stored content = %q", data)
		}
		// storage path must not embed the user-controlled filename
		if strings.Contains(filepath.ToSlash(a.StoragePath), "notes") {
			t.Errorf("storage path uses user filename: %q", a.StoragePath)
		}
	})

	t.Run("associates with a comment on the same issue", func(t *testing.T) {
		f := newFixture(t)
		root, _ := f.svc.AddComment(ctx, "alice", "i1", "", "root")
		a, err := f.svc.AddAttachment(ctx, "alice", "i1", root.ID, "f.bin", "application/octet-stream", strings.NewReader("x"))
		if err != nil {
			t.Fatalf("AddAttachment: %v", err)
		}
		if a.CommentID != root.ID {
			t.Errorf("CommentID = %q, want %q", a.CommentID, root.ID)
		}
	})

	t.Run("unknown comment is 404", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddAttachment(ctx, "alice", "i1", "ghost", "f.bin", "application/octet-stream", strings.NewReader("x"))
		requireStatus(t, err, 404)
	})

	t.Run("comment from another issue is invalid", func(t *testing.T) {
		f := newFixture(t)
		f.comments.byID["C-other"] = &domain.IssueComment{ID: "C-other", IssueID: "i2"}
		_, err := f.svc.AddAttachment(ctx, "alice", "i1", "C-other", "f.bin", "application/octet-stream", strings.NewReader("x"))
		requireStatus(t, err, 400)
	})

	t.Run("rejects empty file name", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddAttachment(ctx, "alice", "i1", "", "  ", "text/plain", strings.NewReader("x"))
		requireStatus(t, err, 400)
	})

	t.Run("enforces the size limit", func(t *testing.T) {
		f := newFixture(t)
		f.svc.MaxAttachmentBytes = 4
		_, err := f.svc.AddAttachment(ctx, "alice", "i1", "", "big.bin", "application/octet-stream", strings.NewReader("12345"))
		requireStatus(t, err, 400)
		// nothing may be left on disk (empty dirs are fine)
		var files int
		_ = filepath.WalkDir(f.dir, func(_ string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files++
			}
			return nil
		})
		if files != 0 {
			t.Errorf("leftover files: %d", files)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.AddAttachment(ctx, "mallory", "i1", "", "f.txt", "text/plain", strings.NewReader("x"))
		requireStatus(t, err, 403)
	})
}

func TestGetAttachmentContent(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the stored bytes", func(t *testing.T) {
		f := newFixture(t)
		a, err := f.svc.AddAttachment(ctx, "alice", "i1", "", "notes.txt", "text/plain", strings.NewReader("hello"))
		if err != nil {
			t.Fatalf("AddAttachment: %v", err)
		}
		got, rc, err := f.svc.GetAttachmentContent(ctx, "bob", "i1", a.ID)
		if err != nil {
			t.Fatalf("GetAttachmentContent: %v", err)
		}
		defer rc.Close()
		data, _ := io.ReadAll(rc)
		if got.ID != a.ID || string(data) != "hello" {
			t.Errorf("got %+v %q", got, data)
		}
	})

	t.Run("unknown attachment is 404", func(t *testing.T) {
		f := newFixture(t)
		_, _, err := f.svc.GetAttachmentContent(ctx, "alice", "i1", "ghost")
		requireStatus(t, err, 404)
	})

	t.Run("attachment from another issue is 404", func(t *testing.T) {
		f := newFixture(t)
		a, _ := f.svc.AddAttachment(ctx, "alice", "i1", "", "f.txt", "text/plain", strings.NewReader("x"))
		_, _, err := f.svc.GetAttachmentContent(ctx, "alice", "other-issue", a.ID)
		requireStatus(t, err, 404)
	})

	t.Run("missing file on disk is internal", func(t *testing.T) {
		f := newFixture(t)
		f.attachments.byID["A-orphan"] = &domain.IssueAttachment{ID: "A-orphan", IssueID: "i1", StoragePath: "nowhere"}
		_, _, err := f.svc.GetAttachmentContent(ctx, "alice", "i1", "A-orphan")
		requireStatus(t, err, 500)
	})
}

func TestListAttachments(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a1, _ := f.svc.AddAttachment(ctx, "alice", "i1", "", "one.txt", "text/plain", strings.NewReader("1"))
	a2, _ := f.svc.AddAttachment(ctx, "bob", "i1", "", "two.txt", "text/plain", strings.NewReader("2"))
	f.attachments.byID["A-noise"] = &domain.IssueAttachment{ID: "A-noise", IssueID: "i2", FileName: "noise"}

	list, err := f.svc.ListAttachments(ctx, "bob", "i1")
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(list) != 2 || list[0].ID != a1.ID || list[1].ID != a2.ID {
		t.Errorf("list = %+v", list)
	}
}

// ---- metadata ----

func TestSetMetadata(t *testing.T) {
	ctx := context.Background()

	t.Run("creates and upserts entries", func(t *testing.T) {
		f := newFixture(t)
		m, err := f.svc.SetMetadata(ctx, "alice", "i1", "env", "staging", "")
		if err != nil {
			t.Fatalf("SetMetadata: %v", err)
		}
		if m.Type != "string" || m.Value != "staging" {
			t.Errorf("entry = %+v", m)
		}
		m, err = f.svc.SetMetadata(ctx, "bob", "i1", "env", "prod", "")
		if err != nil {
			t.Fatalf("SetMetadata upsert: %v", err)
		}
		if m.Value != "prod" {
			t.Errorf("upsert did not overwrite: %+v", m)
		}
		list, _ := f.svc.ListMetadata(ctx, "alice", "i1")
		if len(list) != 1 {
			t.Errorf("entries = %d, want 1", len(list))
		}
	})

	t.Run("number and bool types validate their values", func(t *testing.T) {
		f := newFixture(t)
		m, err := f.svc.SetMetadata(ctx, "alice", "i1", "est_hours", "3.5", "number")
		if err != nil {
			t.Fatalf("SetMetadata number: %v", err)
		}
		if m.Type != "number" || m.Value != "3.5" {
			t.Errorf("entry = %+v", m)
		}
		if _, err := f.svc.SetMetadata(ctx, "alice", "i1", "flag", "yes", "bool"); err == nil {
			t.Error("bool value 'yes' accepted")
		}
		if _, err := f.svc.SetMetadata(ctx, "alice", "i1", "n", "abc", "number"); err == nil {
			t.Error("number value 'abc' accepted")
		}
		if _, err := f.svc.SetMetadata(ctx, "alice", "i1", "x", "v", "bogus"); err == nil {
			t.Error("unknown type accepted")
		}
	})

	t.Run("rejects blank key", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.SetMetadata(ctx, "alice", "i1", "  ", "v", "")
		requireStatus(t, err, 400)
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.SetMetadata(ctx, "mallory", "i1", "k", "v", "")
		requireStatus(t, err, 403)
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.SetMetadata(ctx, "alice", "missing", "k", "v", "")
		requireStatus(t, err, 404)
	})
}

func TestListAndDeleteMetadata(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.svc.SetMetadata(ctx, "alice", "i1", "b_key", "2", "")
	f.svc.SetMetadata(ctx, "alice", "i1", "a_key", "1", "number")
	f.metadata.entries["i2|noise"] = domain.IssueMetadata{IssueID: "i2", Key: "noise", Value: "x", Type: "string"}

	t.Run("lists only the issue's entries sorted by key", func(t *testing.T) {
		list, err := f.svc.ListMetadata(ctx, "bob", "i1")
		if err != nil {
			t.Fatalf("ListMetadata: %v", err)
		}
		if len(list) != 2 || list[0].Key != "a_key" || list[1].Key != "b_key" {
			t.Errorf("list = %+v", list)
		}
	})

	t.Run("deletes an existing key", func(t *testing.T) {
		if err := f.svc.DeleteMetadata(ctx, "alice", "i1", "a_key"); err != nil {
			t.Fatalf("DeleteMetadata: %v", err)
		}
		if err := f.svc.DeleteMetadata(ctx, "alice", "i1", "a_key"); err == nil {
			t.Error("deleting a missing key should fail")
		}
	})

	t.Run("guards access", func(t *testing.T) {
		_, err := f.svc.ListMetadata(ctx, "mallory", "i1")
		requireStatus(t, err, 403)
		err = f.svc.DeleteMetadata(ctx, "mallory", "i1", "b_key")
		requireStatus(t, err, 403)
	})
}
