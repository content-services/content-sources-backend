BEGIN;

alter table templates add column if not exists last_update_snapshot_error VARCHAR(255) DEFAULT NULL;
alter table templates add column if not exists last_update_task_uuid UUID DEFAULT NULL;

COMMIT;
