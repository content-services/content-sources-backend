BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS partner BOOL NOT NULL DEFAULT FALSE;

ALTER TABLE snapshots
    ADD COLUMN IF NOT EXISTS published BOOL NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS repository_configurations_partner_idx
    ON repository_configurations (partner) WHERE partner = true;

CREATE INDEX IF NOT EXISTS snapshots_repo_config_published_idx
    ON snapshots (repository_configuration_uuid, published);

COMMIT;
