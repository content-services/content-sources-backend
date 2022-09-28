BEGIN;

alter table repositories add column package_count int default 0;

COMMIT;
