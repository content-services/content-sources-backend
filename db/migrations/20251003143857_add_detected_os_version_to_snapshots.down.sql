BEGIN;

ALTER TABLE snapshots DROP COLUMN IF EXISTS detected_os_version;

COMMIT;
