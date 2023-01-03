BEGIN;
alter table repositories drop column revision;
COMMIT;
