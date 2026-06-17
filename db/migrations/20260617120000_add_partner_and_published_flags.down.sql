BEGIN;

DROP INDEX IF EXISTS snapshots_repo_config_published_idx;
DROP INDEX IF EXISTS repository_configurations_partner_idx;

ALTER TABLE snapshots DROP COLUMN IF EXISTS published;
ALTER TABLE repository_configurations DROP COLUMN IF EXISTS partner;

COMMIT;
