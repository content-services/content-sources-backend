BEGIN;

alter table snapshots drop constraint if exists fk_repository;
alter table repositories drop column if exists origin;
alter table repositories drop column if exists content_type;

COMMIT;
