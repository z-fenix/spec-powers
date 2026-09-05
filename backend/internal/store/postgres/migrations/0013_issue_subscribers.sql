CREATE TABLE IF NOT EXISTS issue_subscribers (
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_issue_subscribers_user ON issue_subscribers (user_id);
