BEGIN;


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
