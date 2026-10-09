BEGIN;

ALTER TABLE lightwell_package_versions
    ADD COLUMN IF NOT EXISTS upstream_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS project_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS license TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS author TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS author_email TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_lightwell_package_versions_pkg_upstream
    ON lightwell_package_versions (lightwell_package_uuid, upstream_version);

COMMIT;
