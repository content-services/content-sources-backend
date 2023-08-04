BEGIN;

DROP INDEX repo_config_name_deleted_org_id_unique;
ALTER TABLE repository_configurations
    ADD CONSTRAINT name_and_org_id_unique UNIQUE (name, org_id);

DROP INDEX repo_config_repo_org_id_deleted_null_unique;
ALTER TABLE repository_configurations
    ADD CONSTRAINT repo_and_org_id_unique UNIQUE (repository_uuid, org_id);

alter table repository_configurations drop column deleted_at;

alter table snapshots add column repository_uuid UUID;
alter table snapshots add column org_id varchar;

update snapshots set repository_uuid = rc.repository_uuid
from repository_configurations rc
where rc.uuid = snapshots.repository_configuration_uuid;


update snapshots set org_id = rc.org_id
from repository_configurations rc
where rc.uuid = snapshots.repository_configuration_uuid;


alter table snapshots alter column repository_uuid SET NOT NULL;
alter table snapshots alter column org_id SET NOT NULL;


CREATE INDEX IF NOT EXISTS snapshots_org_id_repo_uuid ON snapshots(org_id, repository_uuid);

ALTER TABLE snapshots
    ADD CONSTRAINT fk_repository
        FOREIGN KEY (repository_uuid)
            REFERENCES repositories(uuid);


alter table snapshots drop column repository_configuration_uuid;

COMMIT;
