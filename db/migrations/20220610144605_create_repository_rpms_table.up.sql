
BEGIN;

CREATE TABLE IF NOT EXISTS repository_rpms (
    uuid text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    arch text NOT NULL,
    version text NOT NULL,
    release text,
    epoch integer DEFAULT 0 NOT NULL,
    repo_refer text NOT NULL
);

ALTER TABLE ONLY repository_rpms
    ADD CONSTRAINT repository_rpms_pkey PRIMARY KEY (uuid);

ALTER TABLE ONLY repository_rpms
    ADD CONSTRAINT fk_repository_configurations_packages
    FOREIGN KEY (repo_refer) REFERENCES repository_configurations(uuid)
    ON DELETE CASCADE;

COMMIT;
