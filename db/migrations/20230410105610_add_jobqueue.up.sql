BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS tasks(
                     id uuid PRIMARY KEY,
                     org_id VARCHAR,
                     repository_uuid uuid,
                     token uuid,
                     type VARCHAR NOT NULL,
                     payload jsonb,
                     status VARCHAR,
                     error VARCHAR,
                     queued_at TIMESTAMP WITH TIME ZONE,
                     started_at TIMESTAMP WITH TIME ZONE,
                     finished_at TIMESTAMP WITH TIME ZONE,

                    CONSTRAINT not_finished_when_not_started
                      CHECK (finished_at IS NULL OR started_at IS NOT NULL),

                    CONSTRAINT chronologic_started_at
                      CHECK (started_at IS NULL OR queued_at <= started_at),

                    CONSTRAINT chronologic_finished_at
                      CHECK (finished_at IS NULL OR started_at <= finished_at),

                    CONSTRAINT token_is_never_uuid_nil
                      CHECK (token IS NULL OR token != uuid_nil()),

                    CONSTRAINT token_is_set_when_started
                      CHECK (started_at IS NULL OR token IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS task_dependencies(
                                 task_id uuid REFERENCES tasks(id),
                                 dependency_id uuid REFERENCES tasks(id)
);

CREATE TABLE IF NOT EXISTS task_heartbeats(
                           token uuid PRIMARY KEY,
                           id uuid REFERENCES tasks(id),
                           heartbeat TIMESTAMP WITH TIME ZONE NOT NULL,

                           CONSTRAINT token_is_never_uuid_nil
                               CHECK (token != uuid_nil())
    );

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
ORDER BY queued_at ASC;

ALTER TABLE task_dependencies

DROP CONSTRAINT IF EXISTS task_dependencies_dependency_id_fkey,

DROP CONSTRAINT IF EXISTS task_dependencies_task_id_fkey,

ADD CONSTRAINT task_dependencies_dependency_id_fkey
FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,

ADD CONSTRAINT task_dependencies_task_id_fkey
FOREIGN KEY (dependency_id) REFERENCES tasks(id) ON DELETE CASCADE;

ALTER TABLE task_heartbeats

DROP CONSTRAINT IF EXISTS task_heartbeats_id_fkey,

ADD CONSTRAINT task_heartbeats_id_fkey
FOREIGN KEY (id) REFERENCES tasks(id) ON DELETE CASCADE;

COMMIT;
