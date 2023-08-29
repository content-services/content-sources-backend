BEGIN;

alter table snapshots add column added_counts jsonb NOT NULL  DEFAULT '{}'::jsonb;
alter table snapshots add column removed_counts jsonb NOT NULL  DEFAULT '{}'::jsonb;

alter table repository_configurations add column last_snapshot_uuid UUID DEFAULT NULL;
ALTER TABLE repository_configurations
    ADD CONSTRAINT fk_last_snapshot
        FOREIGN KEY (last_snapshot_uuid)
            REFERENCES snapshots(uuid)
                ON DELETE SET NULL;

COMMIT;
