BEGIN;

ALTER TABLE lightwell_advisories
    DROP COLUMN IF EXISTS upstream;

COMMIT;
