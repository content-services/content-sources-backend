BEGIN;

ALTER TABLE repositories
    ADD COLUMN IF NOT EXISTS security_level VARCHAR(255),
    ADD COLUMN IF NOT EXISTS published_distribution_url VARCHAR(255),
    ADD COLUMN IF NOT EXISTS published_distribution_base_path VARCHAR(255),
    ADD CONSTRAINT repositories_unique_published_dist_url_origin UNIQUE (published_distribution_url, origin);

INSERT INTO domains (org_id, domain_name)
    VALUES ('-3', 'lightwell')
    ON CONFLICT DO NOTHING;

COMMIT;
