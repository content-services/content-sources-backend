BEGIN;

DROP TABLE IF EXISTS clearinghouse_upload_packages;
ALTER TABLE clearinghouse_uploads DROP COLUMN IF EXISTS package_summary;

COMMIT;
