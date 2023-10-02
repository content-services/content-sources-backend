BEGIN;

alter table tasks add column if not exists request_id varchar;

COMMIT;
