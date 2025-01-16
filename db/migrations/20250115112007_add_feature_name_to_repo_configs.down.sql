BEGIN;

DROP INDEX IF EXISTS repo_config_feature_name;
ALTER TABLE repository_configurations DROP COLUMN feature_name;

COMMIT;