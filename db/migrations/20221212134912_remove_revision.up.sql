BEGIN;
alter table repositories drop column if exists revision;
COMMIT;
