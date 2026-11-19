BEGIN;

ALTER TABLE clearinghouse_uploads DROP COLUMN IF EXISTS findings_summary;

COMMIT;
