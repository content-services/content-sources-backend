BEGIN;

DROP INDEX IF EXISTS idx_tasks_ready_status;
DROP INDEX IF EXISTS idx_tasks_priority_queued;
DROP INDEX IF EXISTS idx_tasks_type_ready;
DROP INDEX IF EXISTS idx_task_dependencies_task_id;
DROP INDEX IF EXISTS idx_task_dependencies_dependency_id;
DROP INDEX IF EXISTS idx_tasks_dependency_check;

COMMIT; 
