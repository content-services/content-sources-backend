BEGIN;

DROP TABLE IF EXISTS maven_packages;

CREATE TABLE maven_packages (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    summary TEXT,
    project_url TEXT,
    license TEXT,
    author TEXT
);

COMMIT;
