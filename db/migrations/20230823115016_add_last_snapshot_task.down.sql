BEGIN;

alter table repository_configurations drop column if exists last_snapshot_task_uuid;

COMMIT;
