BEGIN;

ALTER TABLE lightwell_advisories
    DROP COLUMN IF EXISTS fixed_versions;

DROP TABLE IF EXISTS lightwell_advisory_notifications;

COMMIT;
