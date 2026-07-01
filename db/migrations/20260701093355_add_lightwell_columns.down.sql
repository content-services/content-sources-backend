BEGIN;

ALTER TABLE repositories
    DROP CONSTRAINT IF EXISTS repositories_unique_published_dist_url_origin,
    DROP COLUMN IF EXISTS published_distribution_url,
    DROP COLUMN IF EXISTS security_level,
    DROP COLUMN IF EXISTS ecosystem;

COMMIT;
