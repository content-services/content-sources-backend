BEGIN;

ALTER TABLE task_dependencies
DROP CONSTRAINT task_dependencies_dependency_id_fkey,
DROP CONSTRAINT task_dependencies_task_id_fkey;

ALTER TABLE task_heartbeats
DROP CONSTRAINT task_heartbeats_id_fkey;

DROP VIEW IF EXISTS ready_tasks;

DROP TABLE IF EXISTS tasks, task_dependencies, task_heartbeats;

COMMIT;
