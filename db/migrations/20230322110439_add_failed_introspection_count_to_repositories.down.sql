BEGIN;

alter table repositories drop column if exists failed_introspections_count;

COMMIT;
