BEGIN;
ALTER TABLE snapshots DROP COLUMN IF EXISTS publish_task_uuid;
COMMIT;
