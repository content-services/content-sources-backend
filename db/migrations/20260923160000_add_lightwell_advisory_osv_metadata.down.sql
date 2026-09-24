BEGIN;

DROP INDEX IF EXISTS idx_lightwell_advisory_releases_rank;
DROP TABLE IF EXISTS lightwell_advisory_releases;

DROP INDEX IF EXISTS idx_lightwell_advisories_advisory_id_trgm;

ALTER TABLE lightwell_advisories
    ALTER COLUMN repo_name TYPE VARCHAR(255),
    ALTER COLUMN advisory_id TYPE VARCHAR(255),
    ALTER COLUMN severity TYPE VARCHAR(255),
    ALTER COLUMN package_name TYPE VARCHAR(255),
    ALTER COLUMN fixed_version TYPE VARCHAR(255),
    ALTER COLUMN checksum TYPE VARCHAR(255);

ALTER TABLE lightwell_advisories
    DROP COLUMN IF EXISTS package_version,
    DROP COLUMN IF EXISTS summary,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS schema_version,
    DROP COLUMN IF EXISTS aliases,
    DROP COLUMN IF EXISTS modified,
    DROP COLUMN IF EXISTS published;

COMMIT;
