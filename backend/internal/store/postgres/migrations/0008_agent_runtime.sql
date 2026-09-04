CREATE TABLE IF NOT EXISTS agents (
    id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    description text NOT NULL DEFAULT '',
    skills text[] NOT NULL DEFAULT '{}',
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id uuid NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    issue_id uuid NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    trigger_kind text NOT NULL CHECK (trigger_kind IN ('assigned', 'status_changed', 'wakeup', 'manual')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_runs_issue ON runs (issue_id);
CREATE INDEX IF NOT EXISTS idx_runs_agent ON runs (agent_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs (status);

CREATE TABLE IF NOT EXISTS run_logs (
    run_id uuid NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    seq int NOT NULL,
    kind text NOT NULL CHECK (kind IN ('llm_request', 'llm_response', 'tool_call', 'tool_result', 'error')),
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, seq)
);
