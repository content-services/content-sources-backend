BEGIN;

CREATE TABLE IF NOT EXISTS lightwell_advisories (
                                                    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
                                                    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
                                                    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
                                                    repo_name VARCHAR(255) NOT NULL,
                                                    advisory_id VARCHAR(255) NOT NULL,
                                                    severity VARCHAR(255) NOT NULL DEFAULT '',
                                                    details TEXT NOT NULL DEFAULT '',
                                                    reference_urls TEXT[],
                                                    package_name VARCHAR(255) NOT NULL DEFAULT '',
                                                    fixed_version VARCHAR(255) NOT NULL DEFAULT '',
                                                    repository_configuration_uuid UUID NOT NULL,
                                                    checksum VARCHAR(255) NOT NULL
);

ALTER TABLE ONLY lightwell_advisories
    DROP CONSTRAINT IF EXISTS fk_lightwell_advisories_repository_configuration,
    ADD CONSTRAINT fk_lightwell_advisories_repository_configuration
        FOREIGN KEY (repository_configuration_uuid) REFERENCES repository_configurations(uuid) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_lightwell_advisories_repo_config_advisory
    ON lightwell_advisories (repository_configuration_uuid, advisory_id, package_name);

COMMIT;
