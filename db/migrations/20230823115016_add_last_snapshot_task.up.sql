BEGIN;

alter table repository_configurations add column last_snapshot_task_uuid UUID DEFAULT NULL;

COMMIT;
