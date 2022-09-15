BEGIN;

ALTER TABLE repository_configurations
    DROP COLUMN gpg_key,
    DROP COLUMN metadata_verification;

COMMIT;
