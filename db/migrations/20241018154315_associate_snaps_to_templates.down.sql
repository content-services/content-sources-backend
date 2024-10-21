BEGIN;

ALTER TABLE templates_repository_configurations DROP COLUMN IF EXISTS snapshot_uuid;

ALTER TABLE templates_repository_configurations
DROP CONSTRAINT IF EXISTS fk_templates_repository_configurations_snapshots;

COMMIT;