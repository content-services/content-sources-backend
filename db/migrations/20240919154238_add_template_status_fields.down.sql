BEGIN;

alter table templates drop column if exists last_update_snapshot_error;
alter table templates drop column if exists last_update_task_uuid;

COMMIT;
