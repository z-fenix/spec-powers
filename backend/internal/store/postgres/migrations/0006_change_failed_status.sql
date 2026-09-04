ALTER TABLE changes DROP CONSTRAINT changes_status_check;
ALTER TABLE changes ADD CONSTRAINT changes_status_check
    CHECK (status IN ('active', 'archived', 'failed'));
