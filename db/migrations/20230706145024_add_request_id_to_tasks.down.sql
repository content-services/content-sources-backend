BEGIN;

alter table tasks drop column request_id;

COMMIT;
