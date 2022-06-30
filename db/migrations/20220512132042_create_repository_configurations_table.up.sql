BEGIN;

--
-- repositories
--
CREATE TABLE IF NOT EXISTS repositories (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    url VARCHAR(255) NOT NULL,
    last_read_time TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    last_read_error VARCHAR(255) DEFAULT NULL
);

ALTER TABLE repositories
ADD CONSTRAINT repositories_unique_url UNIQUE (url);

--
-- repository_configurations
--
CREATE TABLE IF NOT EXISTS repository_configurations(
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,

    name VARCHAR (255) NOT NULL,
    versions VARCHAR (255)[],
    arch VARCHAR (255) NOT NULL,
    account_id VARCHAR (255) NOT NULL,
    org_id VARCHAR (255) NOT NULL,
    repository_uuid UUID NOT NULL
    );

ALTER TABLE repository_configurations
ADD CONSTRAINT repo_and_org_id_unique UNIQUE (repository_uuid, org_id);

ALTER TABLE repository_configurations
ADD CONSTRAINT fk_repository
FOREIGN KEY (repository_uuid)
REFERENCES repositories(uuid)
ON DELETE SET NULL;

--
-- rpm
--

CREATE TABLE IF NOT EXISTS rpms (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,

    name VARCHAR(255) NOT NULL,
    arch VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    release VARCHAR(255),
    epoch INTEGER DEFAULT 0 NOT NULL,
    summary VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    checksum VARCHAR(255) NOT NULL
);

ALTER TABLE rpms
ADD CONSTRAINT rpms_checksum_unique UNIQUE (checksum);

--
-- repositories_rpms
--
CREATE TABLE repositories_rpms (
    repository_uuid UUID NOT NULL,
    rpm_uuid UUID NOT NULL
);

ALTER TABLE ONLY repositories_rpms
ADD CONSTRAINT repositories_rpms_pkey PRIMARY KEY (repository_uuid, rpm_uuid);

ALTER TABLE ONLY repositories_rpms
ADD CONSTRAINT fk_repositories_rpms_rpm
FOREIGN KEY (rpm_uuid) REFERENCES rpms(uuid)
ON DELETE CASCADE;

ALTER TABLE ONLY repositories_rpms
ADD CONSTRAINT fk_repositories_rpms_repository
FOREIGN KEY (repository_uuid) REFERENCES repositories(uuid)
ON DELETE CASCADE;

ALTER TABLE repository_configurations
ADD CONSTRAINT name_and_org_id_unique UNIQUE (name, org_id);

COMMIT;
