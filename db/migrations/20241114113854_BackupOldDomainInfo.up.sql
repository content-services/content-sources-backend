BEGIN;

alter table domains add column if not exists old_domain_name varchar;
update domains set old_domain_name = domain_name;

alter table snapshots add column if not exists old_version_href varchar;
alter table snapshots add column if not exists old_publication_href varchar;
alter table snapshots add column if not exists old_distribution_href varchar;
alter table snapshots add column if not exists old_repository_path varchar;

update snapshots set old_version_href = version_href;
update snapshots set old_publication_href = publication_href;
update snapshots set old_distribution_href = distribution_href;
update snapshots set old_repository_path = repository_path;

COMMIT;
