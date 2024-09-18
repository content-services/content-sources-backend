BEGIN;

update repository_configurations set last_snapshot_task_uuid = null where last_snapshot_task_uuid not in (select id from tasks);

ALTER TABLE repository_configurations
DROP CONSTRAINT IF EXISTS fk_last_snapshot_task,
ADD CONSTRAINT fk_last_snapshot_task
FOREIGN KEY (last_snapshot_task_uuid)
REFERENCES tasks(id)
ON DELETE SET NULL;
	
COMMIT;
