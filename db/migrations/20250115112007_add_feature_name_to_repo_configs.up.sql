BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS feature_name VARCHAR (255) DEFAULT NULL;

CREATE INDEX IF NOT EXISTS repo_config_feature_name ON repository_configurations(feature_name);

COMMIT;