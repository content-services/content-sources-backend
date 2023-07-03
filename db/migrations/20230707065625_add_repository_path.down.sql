BEGIN;

DROP INDEX snapshots_distribution_path_idx;
ALTER TABLE snapshots DROP COLUMN repository_path;

COMMIT;
