BEGIN;

alter table snapshots drop column if exists added_counts;
alter table snapshots drop column if exists removed_counts;
alter table repository_configurations drop constraint if exists fk_last_snapshot;
alter table repository_configurations drop column if exists latest_snapshot_uuid;

COMMIT;
