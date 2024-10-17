BEGIN;

alter table templates drop column rhsm_environment_created;

COMMIT;
