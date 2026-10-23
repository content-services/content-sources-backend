BEGIN;

-- Dummy rows are not restored. The next migrate up inserts them again when
-- options.seed_lightwell is enabled.

COMMIT;
