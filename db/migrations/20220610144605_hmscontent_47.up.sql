
BEGIN;

--
-- repositories
--
CREATE TABLE IF NOT EXISTS repositories (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    url VARCHAR(255) NOT NULL,
    last_read_time timestamp with time zone,
    last_read_error timestamp with time zone,
    refer2_repo_config UUID NOT NULL
);

--
-- repository_rpms
--
CREATE TABLE IF NOT EXISTS repository_rpms (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name VARCHAR(255) NOT NULL,
    arch VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    release VARCHAR(255),
    epoch integer DEFAULT 0 NOT NULL,
    repo_refer UUID NOT NULL
);

-- ALTER TABLE ONLY repository_rpms
--     ADD CONSTRAINT repository_rpms_pkey PRIMARY KEY (uuid);

ALTER TABLE ONLY repository_rpms
    ADD CONSTRAINT fk_repositories
    FOREIGN KEY (repo_refer) REFERENCES repositories(uuid);

COMMIT;
