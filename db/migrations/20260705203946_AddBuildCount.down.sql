BEGIN;
alter table repositories drop column if exists build_count;
COMMIT;
