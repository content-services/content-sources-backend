BEGIN;
ALTER TABLE snapshots DROP COLUMN IF EXISTS last_publish_task_uuid;
COMMIT;
