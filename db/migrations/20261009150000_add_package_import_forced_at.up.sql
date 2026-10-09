BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS package_import_forced_at TEXT NOT NULL DEFAULT '';

COMMIT;
