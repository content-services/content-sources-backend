BEGIN;

ALTER TABLE repository_configurations
DROP CONSTRAINT IF EXISTS fk_last_snapshot_task;

COMMIT;
