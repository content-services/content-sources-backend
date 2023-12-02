BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS module_hotfixes BOOL DEFAULT FALSE;

COMMIT;
