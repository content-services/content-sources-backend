BEGIN;

CREATE INDEX IF NOT EXISTS tasks_multi_idx ON tasks(org_id, repository_uuid, status, type);

COMMIT;