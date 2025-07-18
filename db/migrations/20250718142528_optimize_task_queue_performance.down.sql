BEGIN;

-- Drop the optimized indexes
DROP INDEX IF EXISTS idx_tasks_ready_status;
DROP INDEX IF EXISTS idx_tasks_priority_queued;
DROP INDEX IF EXISTS idx_tasks_type_ready;
DROP INDEX IF EXISTS idx_task_dependencies_task_id;
DROP INDEX IF EXISTS idx_task_dependencies_dependency_id;
DROP INDEX IF EXISTS idx_tasks_dependency_check;

-- Revert the ready_tasks view to the original version
CREATE OR REPLACE VIEW ready_tasks AS
SELECT *
FROM tasks
WHERE started_at IS NULL
  AND (status != 'canceled' OR status is null)
  AND id NOT IN (
    SELECT task_id
    FROM task_dependencies JOIN tasks ON dependency_id = id
    WHERE finished_at IS NULL
)
ORDER BY priority DESC, queued_at ASC;

COMMIT; 