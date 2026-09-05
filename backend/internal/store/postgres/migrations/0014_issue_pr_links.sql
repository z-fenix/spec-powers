-- Issue keys ("SP-44" style: project key + per-project issue number) and
-- pull-request linkage.

ALTER TABLE projects ADD COLUMN IF NOT EXISTS key text;

-- Key prefixes are unique per workspace; existing rows keep NULL (no key)
-- so the partial index ignores them.
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_workspace_key
    ON projects (workspace_id, key)
    WHERE key IS NOT NULL AND key <> '';

ALTER TABLE issues ADD COLUMN IF NOT EXISTS number bigint;

-- Backfill per-project issue numbers in creation order.
WITH ranked AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at, id) AS rn
    FROM issues
)
UPDATE issues SET number = ranked.rn
FROM ranked
WHERE issues.id = ranked.id AND issues.number IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_issues_project_number
    ON issues (project_id, number)
    WHERE number IS NOT NULL;

CREATE TABLE IF NOT EXISTS pull_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    repo text NOT NULL DEFAULT '',
    number bigint NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    body text NOT NULL DEFAULT '',
    head_branch text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'merged', 'closed')),
    merged_at timestamptz,
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, repo, number)
);

CREATE INDEX IF NOT EXISTS idx_pull_requests_project ON pull_requests (project_id);

CREATE TABLE IF NOT EXISTS issue_pull_requests (
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    pull_request_id uuid NOT NULL REFERENCES pull_requests (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, pull_request_id)
);

CREATE INDEX IF NOT EXISTS idx_issue_pull_requests_pr ON issue_pull_requests (pull_request_id);
