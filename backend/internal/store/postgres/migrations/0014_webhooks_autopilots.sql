CREATE TABLE IF NOT EXISTS webhooks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    secret text NOT NULL CHECK (length(secret) >= 16),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS autopilots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    trigger_type text NOT NULL CHECK (trigger_type IN ('cron', 'webhook', 'manual')),
    cron_spec text NOT NULL DEFAULT '',
    webhook_id uuid REFERENCES webhooks (id) ON DELETE CASCADE,
    action_type text NOT NULL CHECK (action_type IN ('create_issue', 'run_agent')),
    agent_id uuid REFERENCES agents (id) ON DELETE CASCADE,
    project_id uuid REFERENCES projects (id) ON DELETE CASCADE,
    issue_id uuid REFERENCES issues (id) ON DELETE CASCADE,
    issue_title text NOT NULL DEFAULT '',
    issue_description text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES users (id),
    enabled boolean NOT NULL DEFAULT true,
    last_fired_at timestamptz,
    next_run_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_autopilots_webhook ON autopilots (webhook_id);
CREATE INDEX IF NOT EXISTS idx_autopilots_trigger ON autopilots (trigger_type, enabled);
