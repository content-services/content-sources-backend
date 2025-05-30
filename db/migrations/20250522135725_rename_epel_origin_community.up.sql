BEGIN;

ALTER TABLE repositories
    DROP CONSTRAINT IF EXISTS repositories_unique_url,
    DROP CONSTRAINT IF EXISTS repositories_unique_url_origin,
    ADD CONSTRAINT repositories_unique_url_origin UNIQUE (url, origin);

COMMIT;
