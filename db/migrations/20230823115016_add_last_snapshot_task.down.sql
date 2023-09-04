BEGIN;

alter table repository_configurations drop column last_snapshot_task_uuid;

COMMIT;
