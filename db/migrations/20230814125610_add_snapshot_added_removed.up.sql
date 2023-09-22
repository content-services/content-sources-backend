BEGIN;

alter table snapshots add column if not exists added_counts jsonb NOT NULL  DEFAULT '{}'::jsonb;
alter table snapshots add column if not exists removed_counts jsonb NOT NULL  DEFAULT '{}'::jsonb;

alter table repository_configurations add column if not exists last_snapshot_uuid UUID DEFAULT NULL;
ALTER TABLE repository_configurations
    DROP CONSTRAINT IF EXISTS fk_last_snapshot,
    ADD CONSTRAINT fk_last_snapshot
        FOREIGN KEY (last_snapshot_uuid)
            REFERENCES snapshots(uuid)
                ON DELETE SET NULL;

COMMIT;
