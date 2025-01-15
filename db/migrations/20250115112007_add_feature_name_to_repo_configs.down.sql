BEGIN;

ALTER TABLE repository_configurations DROP COLUMN feature_name;

COMMIT;