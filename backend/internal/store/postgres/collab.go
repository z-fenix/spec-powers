package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type CommentStore struct {
	pool *pgxpool.Pool
}

func NewCommentStore(pool *pgxpool.Pool) *CommentStore { return &CommentStore{pool: pool} }

const commentColumns = `
	id, issue_id, COALESCE(parent_id::text, ''), author_id::text, content, created_at`

func scanComment(row pgx.Row) (*domain.IssueComment, error) {
	c := &domain.IssueComment{}
	err := row.Scan(&c.ID, &c.IssueID, &c.ParentID, &c.AuthorID, &c.Content, &c.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan comment: %w", err)
	}
	return c, nil
}

func (s *CommentStore) CreateComment(ctx context.Context, c *domain.IssueComment) (*domain.IssueComment, error) {
	return scanComment(s.pool.QueryRow(ctx, `
		INSERT INTO issue_comments (issue_id, parent_id, author_id, content)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4)
		RETURNING `+commentColumns,
		c.IssueID, c.ParentID, c.AuthorID, c.Content))
}

func (s *CommentStore) GetComment(ctx context.Context, id string) (*domain.IssueComment, error) {
	return scanComment(s.pool.QueryRow(ctx, `
		SELECT `+commentColumns+`
		FROM issue_comments WHERE id = $1`, id))
}

func (s *CommentStore) ListComments(ctx context.Context, issueID string) ([]domain.IssueComment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+commentColumns+`
		FROM issue_comments WHERE issue_id = $1
		ORDER BY created_at, id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()
	var list []domain.IssueComment
	for rows.Next() {
		var c domain.IssueComment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.ParentID, &c.AuthorID, &c.Content, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

type AttachmentStore struct {
	pool *pgxpool.Pool
}

func NewAttachmentStore(pool *pgxpool.Pool) *AttachmentStore { return &AttachmentStore{pool: pool} }

const attachmentColumns = `
	id, issue_id, COALESCE(comment_id::text, ''), file_name, size_bytes,
	content_type, storage_path, uploaded_by::text, created_at`

func scanAttachment(row pgx.Row) (*domain.IssueAttachment, error) {
	a := &domain.IssueAttachment{}
	err := row.Scan(&a.ID, &a.IssueID, &a.CommentID, &a.FileName, &a.SizeBytes,
		&a.ContentType, &a.StoragePath, &a.UploadedBy, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan attachment: %w", err)
	}
	return a, nil
}

func (s *AttachmentStore) CreateAttachment(ctx context.Context, a *domain.IssueAttachment) (*domain.IssueAttachment, error) {
	return scanAttachment(s.pool.QueryRow(ctx, `
		INSERT INTO issue_attachments (issue_id, comment_id, file_name, size_bytes, content_type, storage_path, uploaded_by)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7)
		RETURNING `+attachmentColumns,
		a.IssueID, a.CommentID, a.FileName, a.SizeBytes, a.ContentType, a.StoragePath, a.UploadedBy))
}

func (s *AttachmentStore) GetAttachment(ctx context.Context, id string) (*domain.IssueAttachment, error) {
	return scanAttachment(s.pool.QueryRow(ctx, `
		SELECT `+attachmentColumns+`
		FROM issue_attachments WHERE id = $1`, id))
}

func (s *AttachmentStore) ListAttachments(ctx context.Context, issueID string) ([]domain.IssueAttachment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+attachmentColumns+`
		FROM issue_attachments WHERE issue_id = $1
		ORDER BY created_at, id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	var list []domain.IssueAttachment
	for rows.Next() {
		var a domain.IssueAttachment
		if err := rows.Scan(&a.ID, &a.IssueID, &a.CommentID, &a.FileName, &a.SizeBytes,
			&a.ContentType, &a.StoragePath, &a.UploadedBy, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

type IssueMetadataStore struct {
	pool *pgxpool.Pool
}

func NewIssueMetadataStore(pool *pgxpool.Pool) *IssueMetadataStore {
	return &IssueMetadataStore{pool: pool}
}

func (s *IssueMetadataStore) SetIssueMetadata(ctx context.Context, m *domain.IssueMetadata) (*domain.IssueMetadata, error) {
	out := &domain.IssueMetadata{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO issue_metadata (issue_id, key, value, type)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (issue_id, key) DO UPDATE
		SET value = EXCLUDED.value, type = EXCLUDED.type, updated_at = now()
		RETURNING issue_id, key, value, type, updated_at`,
		m.IssueID, m.Key, m.Value, m.Type).
		Scan(&out.IssueID, &out.Key, &out.Value, &out.Type, &out.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("set issue metadata: %w", err)
	}
	return out, nil
}

func (s *IssueMetadataStore) ListIssueMetadata(ctx context.Context, issueID string) ([]domain.IssueMetadata, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT issue_id, key, value, type, updated_at
		FROM issue_metadata WHERE issue_id = $1
		ORDER BY key`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list issue metadata: %w", err)
	}
	defer rows.Close()
	var list []domain.IssueMetadata
	for rows.Next() {
		var m domain.IssueMetadata
		if err := rows.Scan(&m.IssueID, &m.Key, &m.Value, &m.Type, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan issue metadata: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *IssueMetadataStore) DeleteIssueMetadata(ctx context.Context, issueID, key string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM issue_metadata WHERE issue_id = $1 AND key = $2`, issueID, key)
	if err != nil {
		return fmt.Errorf("delete issue metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}
