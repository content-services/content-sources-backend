BEGIN;

CREATE TABLE IF NOT EXISTS repository_configurations(
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR (255) NOT NULL,
    url VARCHAR (255) NOT NULL,
    versions VARCHAR (255)[],
    arch VARCHAR (255) NOT NULL,
    account_id VARCHAR (255) NOT NULL,
    org_id VARCHAR (255) NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL
    );

ALTER TABLE repository_configurations
ADD CONSTRAINT url_and_org_id_unique UNIQUE (url, org_id);

ALTER TABLE repository_configurations
ADD CONSTRAINT name_and_org_id_unique UNIQUE (name, org_id);

COMMIT;
