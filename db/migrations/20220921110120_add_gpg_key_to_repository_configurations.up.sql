BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS gpg_key TEXT,
    ADD COLUMN IF NOT EXISTS metadata_verification BOOL NOT NULL DEFAULT FALSE;

COMMIT;
