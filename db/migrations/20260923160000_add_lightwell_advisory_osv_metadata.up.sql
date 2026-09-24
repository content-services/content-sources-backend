BEGIN;

ALTER TABLE lightwell_advisories
    ADD COLUMN IF NOT EXISTS published TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS modified TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS aliases TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS schema_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS package_version TEXT NOT NULL DEFAULT '';

ALTER TABLE lightwell_advisories
    ALTER COLUMN repo_name TYPE TEXT,
    ALTER COLUMN advisory_id TYPE TEXT,
    ALTER COLUMN severity TYPE TEXT,
    ALTER COLUMN package_name TYPE TEXT,
    ALTER COLUMN fixed_version TYPE TEXT,
    ALTER COLUMN checksum TYPE TEXT;

CREATE INDEX IF NOT EXISTS idx_lightwell_advisories_advisory_id_trgm
    ON lightwell_advisories USING gin (advisory_id gin_trgm_ops);

CREATE TABLE IF NOT EXISTS lightwell_advisory_releases (
    advisory_uuid UUID NOT NULL REFERENCES lightwell_advisories(uuid) ON DELETE CASCADE,
    release_version TEXT NOT NULL,
    rhlw_baseline INT NOT NULL DEFAULT 0,
    rhlw_novel INT NOT NULL DEFAULT 0,
    rhlw_hotfix INT NOT NULL DEFAULT 0,
    PRIMARY KEY (advisory_uuid, release_version)
);

CREATE INDEX IF NOT EXISTS idx_lightwell_advisory_releases_rank
    ON lightwell_advisory_releases (rhlw_baseline DESC, rhlw_novel DESC, rhlw_hotfix DESC);

COMMIT;
