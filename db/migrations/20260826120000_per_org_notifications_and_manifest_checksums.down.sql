BEGIN;

DROP INDEX IF EXISTS idx_lightwell_advisory_notifications_repo_config_advisory_org;

-- Delete all notifications before dropping org_id column. This is safe because:
-- 1. The old schema doesn't support per-org tracking, so we can't preserve per-org data anyway
-- 2. Notifications are ephemeral tracking - they'll be regenerated on the next sync
-- 3. Without this DELETE, the unique index creation below would fail with duplicate key errors
--    (multiple orgs with same advisory would become duplicate rows after dropping org_id)
DELETE FROM lightwell_advisory_notifications;

ALTER TABLE lightwell_advisory_notifications
    DROP COLUMN IF EXISTS org_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_lightwell_advisory_notifications_repo_config_advisory
    ON lightwell_advisory_notifications (repository_configuration_uuid, advisory_id, package_name);

COMMIT;
