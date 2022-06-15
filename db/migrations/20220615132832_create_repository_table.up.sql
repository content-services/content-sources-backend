BEGIN;

CREATE TABLE IF NOT EXISTS repositories (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    url VARCHAR(255) NOT NULL,
    last_read_time timestamp with time zone,
    last_read_error timestamp with time zone,
    refer2_repo_config UUID NOT NULL
);

ALTER TABLE repositories OWNER TO content;

ALTER TABLE ONLY repositories
    ADD CONSTRAINT repositories_pkey PRIMARY KEY (uuid);

COMMIT;
