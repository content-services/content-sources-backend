BEGIN;

alter table snapshots add column added_counts jsonb NOT NULL  DEFAULT '{}'::jsonb;
alter table snapshots add column removed_counts jsonb NOT NULL  DEFAULT '{}'::jsonb;

COMMIT;
