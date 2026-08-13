BEGIN;

CREATE TABLE IF NOT EXISTS lightwell_advisory_notifications (
                                                    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
                                                    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
                                                    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
                                                    repository_configuration_uuid UUID NOT NULL,
                                                    advisory_id VARCHAR(255) NOT NULL,
                                                    package_name VARCHAR(255) NOT NULL DEFAULT ''
);

ALTER TABLE ONLY lightwell_advisory_notifications
    DROP CONSTRAINT IF EXISTS fk_lightwell_advisory_notifications_repo_config,
    ADD CONSTRAINT fk_lightwell_advisory_notifications_repo_config
        FOREIGN KEY (repository_configuration_uuid) REFERENCES repository_configurations(uuid) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_lightwell_advisory_notifications_repo_config_advisory
    ON lightwell_advisory_notifications (repository_configuration_uuid, advisory_id, package_name);

ALTER TABLE lightwell_advisories
    ADD COLUMN IF NOT EXISTS fixed_versions TEXT[] NOT NULL DEFAULT '{}';

UPDATE lightwell_advisories
    SET fixed_versions = ARRAY[fixed_version]::TEXT[]
    WHERE fixed_version != '';

COMMIT;
