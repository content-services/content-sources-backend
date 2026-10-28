BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS copr_project_id TEXT DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS copr_repository_href TEXT DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_repository_configurations_copr_project_id
    ON repository_configurations(copr_project_id)
    WHERE copr_project_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_repository_configurations_copr_repository_href
    ON repository_configurations(copr_repository_href)
    WHERE copr_repository_href IS NOT NULL;

ALTER TABLE snapshots
    ADD COLUMN IF NOT EXISTS copr_build_id TEXT DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_snapshots_copr_build_id
    ON snapshots(copr_build_id)
    WHERE copr_build_id IS NOT NULL;

COMMIT;
