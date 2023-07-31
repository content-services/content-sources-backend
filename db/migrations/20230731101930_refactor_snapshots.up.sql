BEGIN;

-- we intentionally do not have a foreign key here, so that the deletion workflow 
--  can remove a repo_config while the snapshot deletion happens in the background
alter table snapshots add column repository_configuration_uuid UUID;

update snapshots set repository_configuration_uuid = rc.uuid
    from repository_configurations rc
    where rc.repository_uuid = snapshots.repository_uuid and
          rc.org_id = snapshots.org_id;


alter table snapshots alter column repository_configuration_uuid SET NOT NULL;

CREATE INDEX IF NOT EXISTS snapshots_repo_config_uuid ON snapshots(repository_configuration_uuid);

alter table snapshots drop column repository_uuid;
alter table snapshots drop column org_id;

COMMIT;
