BEGIN;

ALTER TABLE repository_configurations
    DROP COLUMN IF EXISTS package_import_forced_at;

COMMIT;
