BEGIN;

-- Add indexes to support the ready_tasks view efficiently
CREATE INDEX IF NOT EXISTS idx_tasks_ready_status ON tasks(started_at, status) 
WHERE started_at IS NULL AND (status != 'canceled' OR status IS NULL);

CREATE INDEX IF NOT EXISTS idx_tasks_priority_queued ON tasks(priority DESC, queued_at ASC) 
WHERE started_at IS NULL AND (status != 'canceled' OR status IS NULL);

CREATE INDEX IF NOT EXISTS idx_tasks_type_ready ON tasks(type, started_at, status) 
WHERE started_at IS NULL AND (status != 'canceled' OR status IS NULL);

-- Index for task dependencies to optimize the NOT IN subquery
CREATE INDEX IF NOT EXISTS idx_task_dependencies_task_id ON task_dependencies(task_id);
CREATE INDEX IF NOT EXISTS idx_task_dependencies_dependency_id ON task_dependencies(dependency_id);

-- Index for the dependency check in ready_tasks
CREATE INDEX IF NOT EXISTS idx_tasks_dependency_check ON tasks(id, finished_at) 
WHERE finished_at IS NULL;

COMMIT;
