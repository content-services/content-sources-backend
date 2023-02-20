BEGIN;

alter table repositories add column failed_introspections_count int default 0 not null;

COMMIT;
