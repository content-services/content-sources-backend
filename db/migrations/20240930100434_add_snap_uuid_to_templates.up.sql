BEGIN;

ALTER TABLE templates_repository_configurations ADD COLUMN IF NOT EXISTS snapshot_uuid UUID DEFAULT NULL;

ALTER TABLE templates_repository_configurations
DROP CONSTRAINT IF EXISTS fk_templates_repository_configurations_snapshots,
ADD CONSTRAINT fk_templates_repository_configurations_snapshots
FOREIGN KEY (snapshot_uuid) REFERENCES snapshots(uuid)
ON DELETE SET NULL;

COMMIT;