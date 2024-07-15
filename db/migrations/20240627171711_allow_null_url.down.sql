BEGIN;
alter table repositories alter column URL set not null;
COMMIT;
