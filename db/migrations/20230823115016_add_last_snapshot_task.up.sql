BEGIN;

alter table repository_configurations add column if not exists last_snapshot_task_uuid UUID DEFAULT NULL;

COMMIT;
