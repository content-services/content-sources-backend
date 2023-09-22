BEGIN;

alter table repositories add column if not exists failed_introspections_count int default 0 not null;

COMMIT;
