BEGIN;

INSERT INTO repositories (uuid, url, status) VALUES (
    gen_random_uuid(),
    'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/',
    'Pending'
) ON CONFLICT DO NOTHING;
--- Update repo_configs and set the repo_uuid to the new url's repository, only if the repository_config is part of an org that does not also have the new url already
UPDATE repository_configurations SET
    repository_uuid
    = (
        SELECT uuid
        FROM repositories
        WHERE url = 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/'
    )
WHERE repository_configurations.uuid IN (
    SELECT rc.uuid FROM repository_configurations AS rc
    INNER JOIN repositories ON rc.repository_uuid = repositories.uuid
    WHERE
        repositories.url
        = 'https://download-i2.fedoraproject.org/pub/epel/9/Everything/x86_64/'
        AND rc.org_id NOT IN (
            SELECT org_id
            FROM repository_configurations AS rc2
            INNER JOIN repositories AS r2 ON rc2.repository_uuid = r2.uuid
            WHERE
                r2.url
                = 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/'
        )
);

INSERT INTO repositories (uuid, url, status) VALUES (
    gen_random_uuid(),
    'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/',
    'Pending'
) ON CONFLICT DO NOTHING;

UPDATE repository_configurations SET
    repository_uuid
    = (
        SELECT uuid
        FROM repositories
        WHERE url = 'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/'
    )
WHERE repository_configurations.uuid IN (
    SELECT rc.uuid FROM repository_configurations AS rc
    INNER JOIN repositories ON rc.repository_uuid = repositories.uuid
    WHERE
        repositories.url
        = 'https://download-i2.fedoraproject.org/pub/epel/8/Everything/x86_64/'
        AND rc.org_id NOT IN (
            SELECT org_id
            FROM repository_configurations AS rc2
            INNER JOIN repositories AS r2 ON rc2.repository_uuid = r2.uuid
            WHERE
                r2.url
                = 'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/'
        )
);

COMMIT;
