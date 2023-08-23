BEGIN;

alter table snapshots drop column added_counts;
alter table snapshots drop column removed_counts;
alter table repository_configurations drop column latest_snapshot_uuid;

COMMIT;
