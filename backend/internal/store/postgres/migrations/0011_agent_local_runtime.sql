ALTER TABLE agents ADD COLUMN IF NOT EXISTS runtime text NOT NULL DEFAULT 'server'
    CHECK (runtime IN ('server', 'local'));
