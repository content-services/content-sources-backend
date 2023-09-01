BEGIN;

alter table tasks drop column try_cancel;

COMMIT;
