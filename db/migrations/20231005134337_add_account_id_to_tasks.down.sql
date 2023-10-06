BEGIN;

alter table tasks drop column if exists account_id;
DROP INDEX if exists tasks_account_id;

COMMIT;
