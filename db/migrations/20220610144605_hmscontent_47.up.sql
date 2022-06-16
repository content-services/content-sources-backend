BEGIN;

--
-- repositories
--
CREATE TABLE IF NOT EXISTS repositories (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    url VARCHAR(255) NOT NULL,
    last_read_time TIMESTAMP WITH TIME ZONE,
    last_read_error TIMESTAMP WITH TIME ZONE,
    refer_repo_config UUID NOT NULL
);

ALTER TABLE ONLY repositories
ADD CONSTRAINT fk_repositories
FOREIGN KEY (refer_repo_config)
REFERENCES repository_configurations(uuid);

--
-- repository_rpms
--
CREATE TABLE IF NOT EXISTS repository_rpms (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    name VARCHAR(255) NOT NULL,
    arch VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    release VARCHAR(255),
    epoch INTEGER DEFAULT 0 NOT NULL,
    refer_repo UUID NOT NULL
);

-- ALTER TABLE ONLY repository_rpms
--     ADD CONSTRAINT repository_rpms_pkey PRIMARY KEY (uuid);

ALTER TABLE ONLY repository_rpms
ADD CONSTRAINT fk_repositories_rpms
FOREIGN KEY (refer_repo) REFERENCES repositories(uuid)
ON DELETE CASCADE;

COMMIT;
