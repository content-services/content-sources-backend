BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS extended_release VARCHAR(10) DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS extended_release_version VARCHAR(10) DEFAULT NULL;

COMMIT;
