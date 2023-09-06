BEGIN;

alter table repositories drop column origin;
alter table repositories drop column content_type;

COMMIT;
