CREATE TABLE IF NOT EXISTS squads (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    description text NOT NULL DEFAULT '',
    leader_id uuid NOT NULL REFERENCES users (id),
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS squad_members (
    squad_id uuid NOT NULL REFERENCES squads (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (squad_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_squad_members_user ON squad_members (user_id);
