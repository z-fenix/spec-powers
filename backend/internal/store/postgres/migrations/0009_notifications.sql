CREATE TABLE IF NOT EXISTS notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind text NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    body text NOT NULL DEFAULT '',
    issue_id uuid REFERENCES issues (id) ON DELETE SET NULL,
    project_id uuid REFERENCES projects (id) ON DELETE SET NULL,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications (user_id, created_at DESC);
