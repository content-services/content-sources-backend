BEGIN;

alter table repositories add column if not exists package_count int default 0;

COMMIT;
