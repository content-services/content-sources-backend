BEGIN;

ALTER TABLE templates
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS last_updated_by;

COMMIT;
