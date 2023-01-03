BEGIN;
alter table repositories add column repomd_checksum varchar not null default '';
COMMIT;
