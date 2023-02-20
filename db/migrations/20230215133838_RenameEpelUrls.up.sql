BEGIN;

INSERT into repositories (uuid, url) values (gen_random_uuid(), 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/') ON CONFLICT DO NOTHING;
--- Update repo_configs and set the repo_uuid to the new url's repository, only if the repository_config is part of an org that does not also have the new url already
UPDATE repository_configurations set repository_uuid = (select uuid from repositories where url = 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/')
    where repository_configurations.uuid in (
        select rc.uuid from repository_configurations rc
            inner join repositories on repositories.uuid = rc.repository_uuid
            where repositories.url = 'https://download-i2.fedoraproject.org/pub/epel/9/Everything/x86_64/' and
                  rc.org_id not in (select org_id
                                            from repository_configurations rc2
                                            inner join  repositories r2 on r2.uuid = rc2.repository_uuid
                                            where r2.url = 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/'));

INSERT into repositories (uuid, url) values (gen_random_uuid(), 'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/') ON CONFLICT DO NOTHING;

UPDATE repository_configurations set repository_uuid = (select uuid from repositories where url = 'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/')
where repository_configurations.uuid in (
    select rc.uuid from repository_configurations rc
                            inner join repositories on repositories.uuid = rc.repository_uuid
    where repositories.url = 'https://download-i2.fedoraproject.org/pub/epel/8/Everything/x86_64/' and
            rc.org_id not in (select org_id
                              from repository_configurations rc2
                                       inner join  repositories r2 on r2.uuid = rc2.repository_uuid
                              where r2.url = 'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/'));


INSERT into repositories (uuid, url) values (gen_random_uuid(), 'https://dl.fedoraproject.org/pub/epel/7/x86_64/') ON CONFLICT DO NOTHING;
UPDATE repository_configurations set repository_uuid = (select uuid from repositories where url = 'https://dl.fedoraproject.org/pub/epel/7/x86_64/')
where repository_configurations.uuid in (
    select rc.uuid from repository_configurations rc
                            inner join repositories on repositories.uuid = rc.repository_uuid
    where repositories.url = 'https://download-i2.fedoraproject.org/pub/epel/7/x86_64/' and
            rc.org_id not in (select org_id
                              from repository_configurations rc2
                                       inner join  repositories r2 on r2.uuid = rc2.repository_uuid
                              where r2.url = 'https://dl.fedoraproject.org/pub/epel/7/x86_64/'));

COMMIT;
