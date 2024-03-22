BEGIN;
ALTER TABLE templates_repository_configurations
    DROP COLUMN IF EXISTS distribution_href,
    DROP COLUMN IF EXISTS deleted_at;
COMMIT;