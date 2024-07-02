BEGIN;

alter table repositories alter column URL drop not null;

COMMIT;
