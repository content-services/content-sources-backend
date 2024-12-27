BEGIN;

CREATE TABLE IF NOT EXISTS uploads (
    upload_uuid TEXT NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    org_id VARCHAR (255) NOT NULL,
    chunk_size int NOT NULL,
    sha256 TEXT NOT NULL,
    chunk_list TEXT[] default '{}' not null
);

CREATE INDEX IF NOT EXISTS index_orgid_chunksize_sha256 ON uploads(org_id,chunk_size,sha256);

COMMIT;