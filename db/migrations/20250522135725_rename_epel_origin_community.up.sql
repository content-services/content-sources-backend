BEGIN;

ALTER TABLE repositories
    DROP CONSTRAINT IF EXISTS repositories_unique_url,
    DROP CONSTRAINT IF EXISTS repositories_unique_url_origin,
    ADD CONSTRAINT repositories_unique_url_origin UNIQUE (url, origin);

UPDATE repositories SET origin = 'red_hat' WHERE url ILIKE '%https://cdn.redhat.com%' AND origin = 'external';

COMMIT;
