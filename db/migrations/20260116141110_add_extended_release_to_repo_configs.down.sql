BEGIN;

ALTER TABLE repository_configurations DROP COLUMN IF EXISTS extended_release;
ALTER TABLE repository_configurations DROP COLUMN IF EXISTS extended_release_version;

COMMIT;
