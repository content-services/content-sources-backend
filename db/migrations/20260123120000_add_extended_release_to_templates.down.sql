BEGIN;

ALTER TABLE templates
    DROP COLUMN IF EXISTS extended_release,
    DROP COLUMN IF EXISTS extended_release_version;

COMMIT;
