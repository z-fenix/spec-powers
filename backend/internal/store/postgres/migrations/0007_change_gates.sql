-- Guard and archive support: handoff records for phase advances and the
-- verify-report artifact kind.

CREATE TABLE IF NOT EXISTS change_handoffs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    change_id uuid NOT NULL REFERENCES changes (id) ON DELETE CASCADE,
    from_phase text NOT NULL CHECK (from_phase IN ('proposal', 'specs', 'design', 'tasks')),
    to_phase text NOT NULL CHECK (to_phase IN ('specs', 'design', 'tasks')),
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_change_handoffs_change ON change_handoffs (change_id);

ALTER TABLE artifacts DROP CONSTRAINT artifacts_kind_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_kind_check
    CHECK (kind IN ('proposal', 'specs', 'design', 'tasks', 'verify'));
