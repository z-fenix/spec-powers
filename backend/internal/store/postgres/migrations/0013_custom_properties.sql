CREATE TABLE IF NOT EXISTS property_definitions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    type text NOT NULL CHECK (type IN ('select', 'multi_select', 'checkbox', 'text', 'number', 'date')),
    options text[] NOT NULL DEFAULT '{}',
    position int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_property_definitions_project ON property_definitions (project_id);

CREATE TABLE IF NOT EXISTS issue_property_values (
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    property_id uuid NOT NULL REFERENCES property_definitions (id) ON DELETE CASCADE,
    value text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, property_id)
);

CREATE INDEX IF NOT EXISTS idx_issue_property_values_property ON issue_property_values (property_id);
