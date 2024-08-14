BEGIN;

DROP VIEW if exists ready_tasks;

alter table tasks drop column if exists repository_uuid;

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
