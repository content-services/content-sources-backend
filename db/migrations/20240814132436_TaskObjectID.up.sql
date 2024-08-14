BEGIN;

alter table tasks add column if not exists object_uuid UUID;
alter table tasks add column if not exists object_type varchar(255);
update tasks set object_uuid = repository_uuid;

update tasks set object_type = 'repository_config' where object_uuid is not null;

alter table tasks add constraint non_null_obj_type_if_uuid
   CHECK ( ((object_uuid != '00000000-0000-0000-0000-000000000000'::uuid) AND (object_type IS NOT NULL)) 
	OR 
	((object_uuid = '00000000-0000-0000-0000-000000000000'::uuid) AND (object_type IS NULL)) 
    );


DROP INDEX IF EXISTS tasks_multi_idx;
CREATE INDEX IF NOT EXISTS tasks_multi_idx ON tasks(org_id, object_uuid, object_type, status, type);

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
