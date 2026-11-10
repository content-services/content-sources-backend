BEGIN;

-- The Lightwell JavaScript repositories moved from the Pulp base paths
-- npm/validated and npm/remediated to javascript/validated and
-- javascript/remediated. See pkg/external_repos/lightwell_repos.json.
--
-- InternalOnly_RefreshLightwellRepo upserts on (published_distribution_url,
-- origin), so the new base paths produce new repositories and new
-- repository_configurations rows. Nothing in the importer removes entries that
-- have left the allowlist, so the rows under the old names would stay visible
-- to entitled orgs through the Lightwell repositories endpoint, and their
-- advisories would be returned a second time by the advisory list query (which
-- joins repository_configurations without filtering deleted_at). This migration
-- removes them.
--
-- Rows are matched only when all of these hold:
--   * org_id is the Lightwell org (-3)
--   * origin on the joined repository is 'lightwell'
--   * name is exactly one of the two retired names
-- Customer repositories never match: they live in a real org and their origin
-- is 'external' or 'upload'. The demo org (-4) has no npm entries.
--
-- Deleting repository_configurations cascades to lightwell_advisories,
-- lightwell_advisory_notifications, lightwell_packages and
-- lightwell_package_versions. snapshots and
-- templates_repository_configurations reference repository_configurations
-- without ON DELETE CASCADE, so those are cleared first. Both are expected to
-- match zero rows: Lightwell repositories are created with snapshot = false and
-- are not addable to a template.

CREATE TEMPORARY TABLE retired_lightwell_npm_repo_configs ON COMMIT DROP AS
SELECT rc.uuid AS repository_configuration_uuid, rc.repository_uuid
FROM repository_configurations rc
JOIN repositories r ON r.uuid = rc.repository_uuid
WHERE rc.org_id = '-3'
    AND r.origin = 'lightwell'
    AND rc.name IN ('lightwell/npm/validated', 'lightwell/npm/remediated');

DELETE FROM snapshots
WHERE repository_configuration_uuid IN (
    SELECT repository_configuration_uuid FROM retired_lightwell_npm_repo_configs
);

DELETE FROM templates_repository_configurations
WHERE repository_configuration_uuid IN (
    SELECT repository_configuration_uuid FROM retired_lightwell_npm_repo_configs
);

DELETE FROM repository_configurations
WHERE uuid IN (
    SELECT repository_configuration_uuid FROM retired_lightwell_npm_repo_configs
);

-- Drop the repositories rows that are now unreferenced. The repository_uuid is
-- re-checked against repository_configurations so a row shared with another
-- configuration is kept. Without this, repositoryDaoImpl.OrphanCleanup would
-- only reap them a week later.
DELETE FROM repositories
WHERE uuid IN (
    SELECT repository_uuid FROM retired_lightwell_npm_repo_configs
)
    AND NOT EXISTS (
        SELECT 1 FROM repository_configurations rc
        WHERE rc.repository_uuid = repositories.uuid
    );

COMMIT;
