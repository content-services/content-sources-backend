BEGIN;

DROP TABLE IF EXISTS lightwell_package_versions;
DROP TABLE IF EXISTS lightwell_packages;

ALTER TABLE repository_configurations
    DROP COLUMN IF EXISTS last_import_repository_version;

COMMIT;
