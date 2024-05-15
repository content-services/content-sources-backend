BEGIN;

DROP INDEX IF EXISTS repo_config_label_deleted_org_id_unique;

UPDATE repository_configurations
SET label = ''
WHERE org_id != '-1';

COMMIT;
