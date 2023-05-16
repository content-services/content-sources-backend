BEGIN;

DROP VIEW IF EXISTS ready_tasks;

DROP TABLE IF EXISTS tasks, task_dependencies, heartbeats;

COMMIT;
