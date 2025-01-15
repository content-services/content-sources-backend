BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS feature_name VARCHAR (255) DEFAULT NULL ;

COMMIT;