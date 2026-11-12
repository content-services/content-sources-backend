BEGIN;

ALTER TABLE clearinghouse_uploads ADD COLUMN IF NOT EXISTS package_summary JSONB;

CREATE TABLE IF NOT EXISTS clearinghouse_upload_packages (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    clearinghouse_upload_uuid UUID NOT NULL,
    position INTEGER NOT NULL,
    packageurl TEXT NOT NULL,
    ecosystem VARCHAR(255) NOT NULL,
    namespace TEXT,
    name TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '',
    valid_purl BOOLEAN NOT NULL,
    invalid_reason TEXT,
    invalid_detail TEXT
);

ALTER TABLE ONLY clearinghouse_upload_packages
    DROP CONSTRAINT IF EXISTS fk_clearinghouse_upload_packages_clearinghouse_upload,
    ADD CONSTRAINT fk_clearinghouse_upload_packages_clearinghouse_upload
        FOREIGN KEY (clearinghouse_upload_uuid) REFERENCES clearinghouse_uploads(uuid) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_clearinghouse_upload_packages_upload_uuid_position
    ON clearinghouse_upload_packages (clearinghouse_upload_uuid, position);

COMMIT;
