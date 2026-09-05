CREATE TABLE IF NOT EXISTS issue_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    actor_id uuid REFERENCES users (id) ON DELETE SET NULL,
    field text NOT NULL CHECK (length(btrim(field)) > 0),
    old_value text NOT NULL DEFAULT '',
    new_value text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_issue_events_issue ON issue_events (issue_id);
