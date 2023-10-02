BEGIN;

alter table snapshots add column if not exists repository_path varchar default '' not null;

CREATE UNIQUE INDEX IF NOT EXISTS snapshots_distribution_path_idx ON snapshots(distribution_path);

COMMIT;
