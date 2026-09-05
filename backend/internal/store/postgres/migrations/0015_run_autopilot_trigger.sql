ALTER TABLE runs DROP CONSTRAINT runs_trigger_kind_check;
ALTER TABLE runs ADD CONSTRAINT runs_trigger_kind_check
    CHECK (trigger_kind IN ('assigned', 'status_changed', 'wakeup', 'manual', 'mention', 'autopilot'));
