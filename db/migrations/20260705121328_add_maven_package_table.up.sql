BEGIN;

CREATE TABLE IF NOT EXISTS maven_packages (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    summary TEXT,
    project_url TEXT,
    license TEXT,
    author TEXT
);

COMMIT;
