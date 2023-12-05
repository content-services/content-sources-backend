BEGIN;

CREATE TABLE IF NOT EXISTS package_groups (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,

    id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    package_list TEXT[],
    hash_value TEXT
);

CREATE TABLE IF NOT EXISTS repositories_package_groups (
    repository_uuid UUID NOT NULL,
    package_group_uuid UUID NOT NULL
);

ALTER TABLE ONLY repositories_package_groups
DROP CONSTRAINT IF EXISTS repositories_package_groups_pkey,
ADD CONSTRAINT repositories_package_groups_pkey PRIMARY KEY (repository_uuid, package_group_uuid);

ALTER TABLE ONLY repositories_package_groups
DROP CONSTRAINT IF EXISTS fk_repositories_package_groups_pgroup,
ADD CONSTRAINT fk_repositories_package_groups_pgroup
FOREIGN KEY (package_group_uuid) REFERENCES package_groups(uuid)
ON DELETE CASCADE;

ALTER TABLE ONLY repositories_package_groups
DROP CONSTRAINT IF EXISTS fk_repositories_package_groups_repository,
ADD CONSTRAINT fk_repositories_package_groups_repository
FOREIGN KEY (repository_uuid) REFERENCES repositories(uuid)
ON DELETE CASCADE;

COMMIT;