BEGIN;

alter table repository_configurations
    ADD COLUMN IF NOT EXISTS snapshot boolean default false;

CREATE TABLE IF NOT EXISTS snapshots (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    repository_uuid UUID NOT NULL,
    content_counts jsonb NOT NULL  DEFAULT '{}'::jsonb,
    version_href VARCHAR NOT NULL,
    publication_href varchar NOT NULL,
    distribution_path VARCHAR NOT NULL,
    distribution_href VARCHAR NOT NULL,
    org_id varchar NOT NULL
);

-- adding these columns here is only to support rerunning migration after db has been migrated
-- they are removed in a later migration
ALTER TABLE snapshots
ADD COLUMN IF NOT EXISTS org_id varchar,
ADD COLUMN IF NOT EXISTS repository_uuid UUID;


CREATE INDEX IF NOT EXISTS snapshots_org_id_repo_uuid ON snapshots(org_id, repository_uuid);

ALTER TABLE snapshots
    DROP CONSTRAINT IF EXISTS fk_repository,
    ADD CONSTRAINT fk_repository
        FOREIGN KEY (repository_uuid)
            REFERENCES repositories(uuid);


COMMIT;
