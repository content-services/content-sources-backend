BEGIN;

DROP INDEX IF EXISTS idx_snapshots_copr_build_id;
ALTER TABLE snapshots DROP COLUMN IF EXISTS copr_build_id;

DROP INDEX IF EXISTS idx_repository_configurations_copr_repository_href;
DROP INDEX IF EXISTS idx_repository_configurations_copr_project_id;
ALTER TABLE repository_configurations DROP COLUMN IF EXISTS copr_repository_href;
ALTER TABLE repository_configurations DROP COLUMN IF EXISTS copr_project_id;

COMMIT;
