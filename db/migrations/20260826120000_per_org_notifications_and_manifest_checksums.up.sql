BEGIN;

ALTER TABLE lightwell_advisory_notifications
    ADD COLUMN IF NOT EXISTS org_id VARCHAR(255) NOT NULL DEFAULT '';

DELETE FROM lightwell_advisory_notifications WHERE org_id = '';

DROP INDEX IF EXISTS idx_lightwell_advisory_notifications_repo_config_advisory;

CREATE UNIQUE INDEX IF NOT EXISTS idx_lightwell_advisory_notifications_repo_config_advisory_org
    ON lightwell_advisory_notifications (repository_configuration_uuid, advisory_id, package_name, org_id);


COMMIT;
