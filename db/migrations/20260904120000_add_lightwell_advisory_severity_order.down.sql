BEGIN;

DROP INDEX IF EXISTS idx_lightwell_advisories_package_name;
DROP INDEX IF EXISTS idx_lightwell_advisories_severity_score;
ALTER TABLE lightwell_advisories DROP COLUMN IF EXISTS severity_score;

COMMIT;
