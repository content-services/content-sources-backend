BEGIN;

-- update existing custom repository labels, creating duplicates
SELECT * from repository_configurations WHERE label = '' FOR UPDATE;
UPDATE repository_configurations as rc
SET label = regexp_replace(rc.name, '[^a-zA-Z0-9:space]','_','g')
WHERE rc.org_id != '-1';

-- find and update any duplicates, making them unique by adding a suffix
UPDATE repository_configurations as rc
SET label = CONCAT(rc.label, '_', substr(md5(random()::text), 1, 10))
FROM (SELECT count(*) , label FROM repository_configurations GROUP BY label HAVING count(*) > 1)
    AS rc2 WHERE rc.label = rc2.label AND rc.org_id != '-1';

UPDATE repository_configurations as rc
SET label = 'rhel-9-for-aarch64-appstream-rpms'
FROM repositories as r
WHERE rc.repository_uuid = r.uuid AND r.url = 'https://cdn.redhat.com/content/dist/rhel9/9/aarch64/appstream/os/';

CREATE UNIQUE INDEX IF NOT EXISTS repo_config_label_deleted_org_id_unique
ON repository_configurations(label, org_id)
WHERE deleted_at IS NULL;

COMMIT;
