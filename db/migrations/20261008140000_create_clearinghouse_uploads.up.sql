BEGIN;

CREATE TABLE IF NOT EXISTS clearinghouse_uploads (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    org_id VARCHAR(255) NOT NULL,
    account_id TEXT,
    vulnerability_count INTEGER,
    status VARCHAR(255) NOT NULL CHECK (status IN ('validating', 'submitted', 'failed')),
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_clearinghouse_uploads_org_id ON clearinghouse_uploads (org_id);

CREATE TABLE IF NOT EXISTS clearinghouse_upload_files (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    clearinghouse_upload_uuid UUID NOT NULL,
    filename VARCHAR(255) NOT NULL,
    filepath TEXT NOT NULL
);

ALTER TABLE ONLY clearinghouse_upload_files
    DROP CONSTRAINT IF EXISTS fk_clearinghouse_upload_files_clearinghouse_upload,
    ADD CONSTRAINT fk_clearinghouse_upload_files_clearinghouse_upload
        FOREIGN KEY (clearinghouse_upload_uuid) REFERENCES clearinghouse_uploads(uuid) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_clearinghouse_upload_files_upload_uuid_filename
    ON clearinghouse_upload_files (clearinghouse_upload_uuid, filename);

COMMIT;
