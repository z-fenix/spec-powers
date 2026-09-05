ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS run_id uuid REFERENCES runs (id);
