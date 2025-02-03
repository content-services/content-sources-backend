BEGIN;

ALTER TABLE snapshots DROP COLUMN IF EXISTS content_guard_added;

COMMIT;
