BEGIN;
CREATE TABLE IF NOT EXISTS templates (
                                         uuid UUID UNIQUE NOT NULL PRIMARY KEY,
                                         org_id VARCHAR (255) NOT NULL,
                                         created_at TIMESTAMP WITH TIME ZONE,
                                         updated_at TIMESTAMP WITH TIME ZONE,
                                         name VARCHAR (255) NOT NULL,
                                         description VARCHAR (255),
                                         date TIMESTAMP WITH TIME ZONE,
                                         version VARCHAR (255),
                                         arch VARCHAR (255)
);

ALTER TABLE ONLY templates
    DROP CONSTRAINT IF EXISTS name_org_id_unique,
    ADD CONSTRAINT name_org_id_unique unique(org_id, name);

CREATE TABLE IF NOT EXISTS templates_repository_configurations(
                                                                  template_uuid UUID NOT NULL,
                                                                  repository_configuration_uuid UUID NOT NULL
);

ALTER TABLE ONLY templates_repository_configurations
    DROP CONSTRAINT IF EXISTS templates_repository_configurations_pkey,
    ADD CONSTRAINT templates_repository_configurations_pkey PRIMARY KEY (template_uuid, repository_configuration_uuid);

ALTER TABLE ONLY templates_repository_configurations
    DROP CONSTRAINT IF EXISTS template_uuid_repository_configuration_uuid,
    ADD CONSTRAINT template_uuid_repository_configuration_uuid unique(template_uuid, repository_configuration_uuid);

ALTER TABLE ONLY templates_repository_configurations
    DROP CONSTRAINT IF EXISTS fk_templates_repository_configurations_templates,
    ADD CONSTRAINT fk_templates_repository_configurations_templates
        FOREIGN KEY (template_uuid) REFERENCES templates(uuid)
            ON DELETE CASCADE;

ALTER TABLE ONLY templates_repository_configurations
    DROP CONSTRAINT IF EXISTS fk_templates_repository_configurations_repositories,
    ADD CONSTRAINT fk_templates_repository_configurations_repositories
        FOREIGN KEY (repository_configuration_uuid) REFERENCES repository_configurations(uuid)
            ON DELETE CASCADE;

COMMIT;
