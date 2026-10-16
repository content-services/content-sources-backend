BEGIN;

ALTER TABLE coverage_reports ADD COLUMN IF NOT EXISTS skipped_entries INTEGER;

COMMIT;
