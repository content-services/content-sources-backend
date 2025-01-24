BEGIN;

ALTER TABLE repository_configurations ADD COLUMN IF NOT EXISTS failed_snapshot_count int  default 0 NOT NULL;

COMMIT;
