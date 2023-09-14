BEGIN;

alter table repositories drop column if exists origin;
alter table repositories drop column if exists content_type;

COMMIT;
