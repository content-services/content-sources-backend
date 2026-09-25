BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS last_import_repository_version TEXT;

CREATE TABLE lightwell_packages (
    uuid UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    package_group TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_packages_repo_group_name
    ON lightwell_packages (repository_configuration_uuid, package_group, name);

CREATE TABLE lightwell_package_versions (
    uuid UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lightwell_package_uuid UUID NOT NULL REFERENCES lightwell_packages(uuid) ON DELETE CASCADE,
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    version TEXT NOT NULL,
    release TEXT NOT NULL DEFAULT '',
    published_at TEXT NOT NULL DEFAULT '',
    purl TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_package_versions_pkg_version
    ON lightwell_package_versions (lightwell_package_uuid, version);

CREATE INDEX idx_lightwell_package_versions_repo
    ON lightwell_package_versions (repository_configuration_uuid);

COMMIT;
