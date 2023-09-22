BEGIN;

alter table repositories drop column if exists package_count;

COMMIT;
