BEGIN;

-- we intentionally do not have a foreign key here, so that the deletion workflow 
--  can remove a repo_config while the snapshot deletion happens in the background
alter table snapshots add column if not exists repository_configuration_uuid UUID;
ALTER TABLE snapshots
    DROP CONSTRAINT IF EXISTS fk_snapshots_repo_config_uuid,
    ADD CONSTRAINT fk_snapshots_repo_config_uuid
        FOREIGN KEY (repository_configuration_uuid)
            REFERENCES repository_configurations(uuid)
            ON DELETE SET NULL;

update snapshots set repository_configuration_uuid = rc.uuid
    from repository_configurations rc
    where rc.repository_uuid = snapshots.repository_uuid and
          rc.org_id = snapshots.org_id;


alter table snapshots alter column repository_configuration_uuid SET NOT NULL;

CREATE INDEX IF NOT EXISTS snapshots_repo_config_uuid ON snapshots(repository_configuration_uuid);

alter table snapshots drop column repository_uuid;
alter table snapshots drop column org_id;

alter table repository_configurations add column if not exists deleted_at TIMESTAMP WITH TIME ZONE;


ALTER TABLE repository_configurations
    DROP CONSTRAINT name_and_org_id_unique;

CREATE UNIQUE INDEX IF NOT EXISTS repo_config_name_deleted_org_id_unique
    ON repository_configurations(name, org_id)
    WHERE deleted_at IS NULL;

ALTER TABLE repository_configurations
    DROP CONSTRAINT repo_and_org_id_unique;
CREATE UNIQUE INDEX IF NOT EXISTS repo_config_repo_org_id_deleted_null_unique
    ON repository_configurations(repository_uuid, org_id)
    WHERE deleted_at IS NULL;


COMMIT;
