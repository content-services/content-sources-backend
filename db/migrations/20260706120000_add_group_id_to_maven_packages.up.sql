BEGIN;

DROP TABLE IF EXISTS maven_packages;

CREATE TABLE maven_packages (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    group_id TEXT NOT NULL,
    name TEXT NOT NULL,
    summary TEXT,
    project_url TEXT,
    license TEXT,
    author TEXT
);

ALTER TABLE maven_packages
DROP CONSTRAINT IF EXISTS maven_packages_name_group_id_unique,
ADD CONSTRAINT maven_packages_name_group_id_unique UNIQUE (group_id, name);

COMMIT;
