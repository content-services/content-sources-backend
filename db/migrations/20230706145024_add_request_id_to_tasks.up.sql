BEGIN;

alter table tasks add column request_id varchar;

COMMIT;
