BEGIN;

ALTER TABLE uploads DROP COLUMN IF EXISTS size;

COMMIT;
