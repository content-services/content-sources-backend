BEGIN;

ALTER TABLE repository_configurations DROP COLUMN IF EXISTS module_hotfixes;

COMMIT;
