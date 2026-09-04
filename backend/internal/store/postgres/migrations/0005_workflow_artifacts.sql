CREATE TABLE IF NOT EXISTS changes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    issue_id uuid NOT NULL UNIQUE REFERENCES issues (id) ON DELETE CASCADE,
    phase text NOT NULL DEFAULT 'proposal' CHECK (phase IN ('proposal', 'specs', 'design', 'tasks')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_changes_project ON changes (project_id);

CREATE TABLE IF NOT EXISTS artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    change_id uuid NOT NULL REFERENCES changes (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('proposal', 'specs', 'design', 'tasks')),
    version int NOT NULL CHECK (version > 0),
    content text NOT NULL,
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (change_id, kind, version)
);

CREATE INDEX IF NOT EXISTS idx_artifacts_change ON artifacts (change_id);

CREATE TABLE IF NOT EXISTS task_mappings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    change_id uuid NOT NULL REFERENCES changes (id) ON DELETE CASCADE,
    artifact_id uuid NOT NULL REFERENCES artifacts (id) ON DELETE CASCADE,
    issue_id uuid NOT NULL UNIQUE REFERENCES issues (id) ON DELETE CASCADE,
    title text NOT NULL DEFAULT '',
    stage int NOT NULL DEFAULT 0,
    position int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_task_mappings_change ON task_mappings (change_id);
