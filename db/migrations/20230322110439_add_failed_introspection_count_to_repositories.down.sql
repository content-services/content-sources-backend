BEGIN;

alter table repositories drop column failed_introspections_count;

COMMIT;
