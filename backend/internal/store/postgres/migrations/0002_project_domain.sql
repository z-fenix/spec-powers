ALTER TABLE projects ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS archived boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS project_resources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('github_repo', 'local_directory')),
    label text NOT NULL,
    pointer text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, type, pointer)
);

CREATE TABLE IF NOT EXISTS project_contexts (
    project_id uuid PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    content text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_project_resources_project ON project_resources (project_id);
