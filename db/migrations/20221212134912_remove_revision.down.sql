BEGIN;
alter table repositories add column revision varchar(255);
COMMIT;
