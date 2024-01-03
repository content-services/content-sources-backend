BEGIN;

CREATE TABLE IF NOT EXISTS environments (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,

    id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT
);

CREATE TABLE IF NOT EXISTS repositories_environments (
    repository_uuid UUID NOT NULL,
    environment_uuid UUID NOT NULL
);

ALTER TABLE ONLY repositories_environments
DROP CONSTRAINT IF EXISTS repositories_environments_pkey,
ADD CONSTRAINT repositories_environments_pkey PRIMARY KEY (repository_uuid, environment_uuid);

ALTER TABLE ONLY repositories_environments
DROP CONSTRAINT IF EXISTS fk_repositories_environments_env,
ADD CONSTRAINT fk_repositories_environments_env
FOREIGN KEY (environment_uuid) REFERENCES environments(uuid)
ON DELETE CASCADE;

ALTER TABLE ONLY repositories_environments
DROP CONSTRAINT IF EXISTS fk_repositories_environments_repository,
ADD CONSTRAINT fk_repositories_environments_repository
FOREIGN KEY (repository_uuid) REFERENCES repositories(uuid)
ON DELETE CASCADE;

COMMIT;