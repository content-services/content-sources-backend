BEGIN;
alter table repositories add column if not exists revision varchar(255);
COMMIT;
