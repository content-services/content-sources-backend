BEGIN;

ALTER TABLE clearinghouse_uploads ADD COLUMN IF NOT EXISTS findings_summary JSONB;

COMMIT;
