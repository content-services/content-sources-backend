BEGIN;

alter table domains drop column if exists old_domain_name;
alter table snapshots drop column if exists old_version_href;
alter table snapshots drop column if exists old_publication_href;
alter table snapshots drop column if exists old_distribution_href;
alter table snapshots drop column if exists old_repository_path;

COMMIT;
