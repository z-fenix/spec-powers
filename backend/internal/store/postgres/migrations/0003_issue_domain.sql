CREATE TABLE IF NOT EXISTS issues (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    parent_id uuid REFERENCES issues (id) ON DELETE CASCADE,
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'todo' CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'blocked', 'cancelled')),
    priority text NOT NULL DEFAULT 'none' CHECK (priority IN ('none', 'low', 'medium', 'high', 'urgent')),
    assignee_id uuid REFERENCES users (id) ON DELETE SET NULL,
    due_date date,
    labels text[] NOT NULL DEFAULT '{}',
    stage int NOT NULL DEFAULT 0,
    position int NOT NULL DEFAULT 0,
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_issues_project ON issues (project_id);
CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues (parent_id);

CREATE TABLE IF NOT EXISTS issue_wakeups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    child_issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issue_id, child_issue_id)
);
