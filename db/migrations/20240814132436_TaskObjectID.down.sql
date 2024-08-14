BEGIN;

DROP VIEW if exists ready_tasks;

DROP INDEX IF EXISTS tasks_multi_idx;
CREATE INDEX IF NOT EXISTS tasks_multi_idx ON tasks(org_id, repository_uuid, status, type);

alter table tasks drop constraint non_null_obj_type_if_uuid;
alter table tasks drop column if exists object_uuid;
alter table tasks drop column if exists object_type;

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
