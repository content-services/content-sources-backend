BEGIN;

DROP INDEX IF EXISTS idx_lightwell_package_versions_pkg_upstream;

ALTER TABLE lightwell_package_versions
    DROP COLUMN IF EXISTS upstream_version,
    DROP COLUMN IF EXISTS project_url,
    DROP COLUMN IF EXISTS license,
    DROP COLUMN IF EXISTS summary,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS author,
    DROP COLUMN IF EXISTS author_email;

COMMIT;
