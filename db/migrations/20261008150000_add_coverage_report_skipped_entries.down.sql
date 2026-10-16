BEGIN;

ALTER TABLE coverage_reports DROP COLUMN IF EXISTS skipped_entries;

COMMIT;
