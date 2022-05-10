BEGIN;

CREATE TABLE IF NOT EXISTS repository_configurations(
    uuid VARCHAR (255) UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR (255) NOT NULL,
    url VARCHAR (255) NOT NULL,
    version VARCHAR (255),
    arch VARCHAR (255),
    account_id VARCHAR (255) NOT NULL,
    org_id VARCHAR (255) NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL
    );

COMMIT;