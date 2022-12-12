BEGIN;
alter table repositories drop column repomd_checksum;
COMMIT;
