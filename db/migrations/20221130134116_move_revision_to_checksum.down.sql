BEGIN;
alter table repositories drop column if exists repomd_checksum;
COMMIT;
