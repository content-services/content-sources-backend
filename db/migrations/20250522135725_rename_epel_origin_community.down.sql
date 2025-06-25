BEGIN;

UPDATE repositories SET origin = 'external' WHERE url ILIKE '%https://cdn.redhat.com%' AND origin = 'red_hat';

DELETE FROM repository_configurations
WHERE repository_uuid IN (
    SELECT uuid
    FROM repositories
    WHERE origin = 'community'
      AND url IN (
        SELECT url
        FROM repositories
        GROUP BY url
        HAVING COUNT(*) > 1
    )
);

DELETE FROM repositories
WHERE origin = 'community'
  AND url IN (
    SELECT url
    FROM repositories
    GROUP BY url
    HAVING COUNT(*) > 1
);

ALTER TABLE repositories
    DROP CONSTRAINT IF EXISTS repositories_unique_url_origin,
    DROP CONSTRAINT IF EXISTS repositories_unique_url,
    ADD CONSTRAINT repositories_unique_url UNIQUE (url);

COMMIT;
