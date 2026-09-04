package collab

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// MaxAttachmentBytes is the default per-file upload cap.
const MaxAttachmentBytes int64 = 20 << 20

var metadataTypes = map[string]func(string) bool{
	"string": func(string) bool { return true },
	"number": func(v string) bool { _, err := strconv.ParseFloat(v, 64); return err == nil },
	"bool":   func(v string) bool { return v == "true" || v == "false" },
}

// issueLookup and projectAccess are the slices of the issue/project stores
// the collab domain needs.
type issueLookup interface {
	GetIssue(ctx context.Context, id string) (*domain.Issue, error)
}

type projectAccess interface {
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
}

type Service struct {
	issues      issueLookup
	projects    projectAccess
	comments    store.CommentStore
	attachments store.AttachmentStore
	metadata    store.IssueMetadataStore

	// AttachmentDir is the local directory files are stored under;
	// StoragePath values are relative to it.
	AttachmentDir string
	// MaxAttachmentBytes caps one upload; tests may lower it.
	MaxAttachmentBytes int64
}

func NewService(issues issueLookup, projects projectAccess, comments store.CommentStore, attachments store.AttachmentStore, metadata store.IssueMetadataStore, attachmentDir string) *Service {
	return &Service{
		issues:             issues,
		projects:           projects,
		comments:           comments,
		attachments:        attachments,
		metadata:           metadata,
		AttachmentDir:      attachmentDir,
		MaxAttachmentBytes: MaxAttachmentBytes,
	}
}

// requireProjectIssue mirrors the issue domain's access rule: the issue must
// exist and the caller must be at least a project member.
func (s *Service) requireProjectIssue(ctx context.Context, userID, issueID string) (*domain.Issue, error) {
	i, err := s.issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get issue failed")
	}
	if _, err := s.projects.GetProject(ctx, i.ProjectID); err != nil {
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("project not found")
		}
		return nil, httpapi.ErrInternal("get project failed")
	}
	pm, err := s.projects.GetProjectMember(ctx, i.ProjectID, userID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrForbidden("not a project member")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get project member failed")
	}
	if pm.Role != "owner" && pm.Role != "member" {
		return nil, httpapi.ErrForbidden("not a project member")
	}
	return i, nil
}

// AddComment creates a root comment, or a reply when parentCommentID is set.
// Threads are single-level: the parent must be a root comment on the same
// issue.
func (s *Service) AddComment(ctx context.Context, userID, issueID, parentCommentID, content string) (*domain.IssueComment, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, httpapi.ErrInvalid("comment content is required")
	}
	parentID := ""
	if parentCommentID != "" {
		parent, err := s.comments.GetComment(ctx, parentCommentID)
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("parent comment not found")
		}
		if err != nil {
			return nil, httpapi.ErrInternal("get parent comment failed")
		}
		if parent.IssueID != issueID {
			return nil, httpapi.ErrInvalid("parent comment belongs to another issue")
		}
		if parent.ParentID != "" {
			return nil, httpapi.ErrInvalid("cannot reply to a reply")
		}
		parentID = parent.ID
	}
	c, err := s.comments.CreateComment(ctx, &domain.IssueComment{
		IssueID:  issueID,
		ParentID: parentID,
		AuthorID: userID,
		Content:  content,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("create comment failed")
	}
	return c, nil
}

func (s *Service) ListComments(ctx context.Context, userID, issueID string) ([]domain.IssueComment, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	list, err := s.comments.ListComments(ctx, issueID)
	if err != nil {
		return nil, httpapi.ErrInternal("list comments failed")
	}
	return list, nil
}

// AddAttachment stores the file under the attachment directory (path derived
// from the generated ID, never the user-supplied name) and records its
// metadata. The file is removed again if the database insert fails.
func (s *Service) AddAttachment(ctx context.Context, userID, issueID, commentID, fileName, contentType string, r io.Reader) (*domain.IssueAttachment, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(fileName) == "" {
		return nil, httpapi.ErrInvalid("file name is required")
	}
	commentRef := ""
	if commentID != "" {
		c, err := s.comments.GetComment(ctx, commentID)
		if err == store.ErrNotFound {
			return nil, httpapi.ErrNotFound("comment not found")
		}
		if err != nil {
			return nil, httpapi.ErrInternal("get comment failed")
		}
		if c.IssueID != issueID {
			return nil, httpapi.ErrInvalid("comment belongs to another issue")
		}
		commentRef = c.ID
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id, err := newID()
	if err != nil {
		return nil, httpapi.ErrInternal("generate attachment id failed")
	}
	relPath := filepath.ToSlash(filepath.Join(issueID, id))
	fullPath := filepath.Join(s.AttachmentDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, httpapi.ErrInternal("prepare attachment storage failed")
	}
	size, err := writeLimited(fullPath, r, s.MaxAttachmentBytes)
	if err != nil {
		return nil, err.(*httpapi.AppError)
	}
	a, err := s.attachments.CreateAttachment(ctx, &domain.IssueAttachment{
		IssueID:     issueID,
		CommentID:   commentRef,
		FileName:    fileName,
		SizeBytes:   size,
		ContentType: contentType,
		StoragePath: relPath,
		UploadedBy:  userID,
	})
	if err != nil {
		_ = os.Remove(fullPath)
		return nil, httpapi.ErrInternal("create attachment failed")
	}
	return a, nil
}

// writeLimited copies r to path, failing once the cap is exceeded so no
// unbounded upload lands on disk. The file is removed on any error.
func writeLimited(path string, r io.Reader, cap int64) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, httpapi.ErrInternal("create attachment file failed")
	}
	size, err := io.Copy(f, io.LimitReader(r, cap+1))
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && size > cap {
		err = httpapi.ErrInvalid("attachment exceeds the maximum size")
	}
	if err != nil {
		_ = os.Remove(path)
		return 0, err
	}
	return size, nil
}

func (s *Service) GetAttachmentContent(ctx context.Context, userID, issueID, attachmentID string) (*domain.IssueAttachment, io.ReadCloser, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, nil, err
	}
	a, err := s.attachments.GetAttachment(ctx, attachmentID)
	if err == store.ErrNotFound {
		return nil, nil, httpapi.ErrNotFound("attachment not found")
	}
	if err != nil {
		return nil, nil, httpapi.ErrInternal("get attachment failed")
	}
	if a.IssueID != issueID {
		return nil, nil, httpapi.ErrNotFound("attachment not found")
	}
	f, err := os.Open(filepath.Join(s.AttachmentDir, filepath.FromSlash(a.StoragePath)))
	if err != nil {
		return nil, nil, httpapi.ErrInternal("attachment file missing")
	}
	return a, f, nil
}

func (s *Service) ListAttachments(ctx context.Context, userID, issueID string) ([]domain.IssueAttachment, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	list, err := s.attachments.ListAttachments(ctx, issueID)
	if err != nil {
		return nil, httpapi.ErrInternal("list attachments failed")
	}
	return list, nil
}

// SetMetadata upserts one KV entry. An empty type means "string".
func (s *Service) SetMetadata(ctx context.Context, userID, issueID, key, value, mtype string) (*domain.IssueMetadata, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, httpapi.ErrInvalid("metadata key is required")
	}
	if mtype == "" {
		mtype = "string"
	}
	check, ok := metadataTypes[mtype]
	if !ok {
		return nil, httpapi.ErrInvalid("unknown metadata type: " + mtype)
	}
	if !check(value) {
		return nil, httpapi.ErrInvalid(fmt.Sprintf("invalid %s value: %q", mtype, value))
	}
	m, err := s.metadata.SetIssueMetadata(ctx, &domain.IssueMetadata{
		IssueID: issueID,
		Key:     key,
		Value:   value,
		Type:    mtype,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("set metadata failed")
	}
	return m, nil
}

func (s *Service) ListMetadata(ctx context.Context, userID, issueID string) ([]domain.IssueMetadata, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	list, err := s.metadata.ListIssueMetadata(ctx, issueID)
	if err != nil {
		return nil, httpapi.ErrInternal("list metadata failed")
	}
	return list, nil
}

func (s *Service) DeleteMetadata(ctx context.Context, userID, issueID, key string) error {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return err
	}
	if err := s.metadata.DeleteIssueMetadata(ctx, issueID, key); err == store.ErrNotFound {
		return httpapi.ErrNotFound("metadata key not found")
	} else if err != nil {
		return httpapi.ErrInternal("delete metadata failed")
	}
	return nil
}

// newID returns a random RFC 4122 v4 UUID string.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
