BEGIN;

alter table repositories drop column package_count;

COMMIT;
