CREATE TABLE IF NOT EXISTS issue_comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    parent_id uuid REFERENCES issue_comments (id) ON DELETE CASCADE,
    author_id uuid NOT NULL REFERENCES users (id),
    content text NOT NULL CHECK (length(btrim(content)) > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_issue_comments_issue ON issue_comments (issue_id);
CREATE INDEX IF NOT EXISTS idx_issue_comments_parent ON issue_comments (parent_id);

CREATE TABLE IF NOT EXISTS issue_attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    comment_id uuid REFERENCES issue_comments (id) ON DELETE CASCADE,
    file_name text NOT NULL CHECK (length(btrim(file_name)) > 0),
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    content_type text NOT NULL DEFAULT 'application/octet-stream',
    storage_path text NOT NULL,
    uploaded_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_issue_attachments_issue ON issue_attachments (issue_id);
CREATE INDEX IF NOT EXISTS idx_issue_attachments_comment ON issue_attachments (comment_id);

CREATE TABLE IF NOT EXISTS issue_metadata (
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    key text NOT NULL CHECK (length(btrim(key)) > 0),
    value text NOT NULL,
    type text NOT NULL DEFAULT 'string' CHECK (type IN ('string', 'number', 'bool')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, key)
);
