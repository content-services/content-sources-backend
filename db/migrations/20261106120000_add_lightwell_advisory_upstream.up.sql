BEGIN;

ALTER TABLE lightwell_advisories
    ADD COLUMN IF NOT EXISTS upstream TEXT[] NOT NULL DEFAULT '{}';

UPDATE lightwell_advisories
SET upstream = aliases
WHERE upstream = '{}';

COMMIT;
