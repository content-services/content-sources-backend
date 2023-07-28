BEGIN;

alter table snapshots drop column added_counts;
alter table snapshots drop column removed_counts;

COMMIT;
