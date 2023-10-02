BEGIN;
alter table repositories add column if not exists repomd_checksum varchar not null default '';
COMMIT;
