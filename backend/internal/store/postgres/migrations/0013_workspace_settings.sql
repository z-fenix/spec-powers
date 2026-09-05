CREATE TABLE IF NOT EXISTS workspace_invites (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email citext NOT NULL,
    role_id smallint NOT NULL REFERENCES roles (id),
    code text NOT NULL UNIQUE,
    invited_by uuid NOT NULL REFERENCES users (id),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz
);

-- One live invitation per (workspace, email); superseded ones keep history.
CREATE UNIQUE INDEX IF NOT EXISTS uq_workspace_invites_pending
    ON workspace_invites (workspace_id, email) WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_workspace_invites_workspace ON workspace_invites (workspace_id, created_at DESC);

CREATE TABLE IF NOT EXISTS api_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    prefix text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens (user_id, created_at DESC);
