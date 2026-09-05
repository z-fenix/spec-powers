ALTER TABLE project_resources DROP CONSTRAINT IF EXISTS project_resources_type_check;
ALTER TABLE project_resources ADD CONSTRAINT project_resources_type_check
    CHECK (type IN ('github_repo', 'local_directory', 'worktree'));
ALTER TABLE project_resources ADD COLUMN IF NOT EXISTS branch text NOT NULL DEFAULT '';
ALTER TABLE project_resources ADD COLUMN IF NOT EXISTS path text NOT NULL DEFAULT '';
