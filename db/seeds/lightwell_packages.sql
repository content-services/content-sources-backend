-- Dev seed for the Lightwell package mirror.
-- Apply db/seeds/lightwell_advisories.sql first. Packages are written onto
-- the repositories created by scripts/create_lightwell_repo.sh.
--
-- Remediated versions are the advisory fixed_versions strings, including rhlw.
-- Validated versions are the upstream part of those strings, with an empty release.
-- Maven name is the artifact id and package_group is the group id. The
-- group:artifact coordinate stays on the advisory and is not stored in name.
-- JavaScript and C# advisories stay on lightwell/seed, and there is no npm repo.
BEGIN;

DELETE FROM lightwell_package_versions
WHERE repository_configuration_uuid IN (
    SELECT uuid FROM repository_configurations
    WHERE org_id = '-3'
        AND deleted_at IS NULL
        AND name IN (
            'lightwell/java/validated',
            'lightwell/java/remediated',
            'lightwell/python/validated',
            'lightwell/python/remediated'
        )
);

DELETE FROM lightwell_packages
WHERE repository_configuration_uuid IN (
    SELECT uuid FROM repository_configurations
    WHERE org_id = '-3'
        AND deleted_at IS NULL
        AND name IN (
            'lightwell/java/validated',
            'lightwell/java/remediated',
            'lightwell/python/validated',
            'lightwell/python/remediated'
        )
);

CREATE TEMP TABLE seed_package_versions ON COMMIT DROP AS
WITH seed_advisories AS (
    SELECT
        la.repository_configuration_uuid AS remediated_uuid,
        validated.uuid AS validated_uuid,
        r.content_type,
        CASE
            WHEN r.content_type = 'maven' THEN split_part(la.package_name, ':', 1)
            ELSE ''
        END AS package_group,
        CASE
            WHEN r.content_type = 'maven' THEN split_part(la.package_name, ':', 2)
            ELSE la.package_name
        END AS package_name,
        ver AS stored_version,
        la.published
    FROM lightwell_advisories la
    JOIN repository_configurations rc ON rc.uuid = la.repository_configuration_uuid
    JOIN repositories r ON r.uuid = rc.repository_uuid
    JOIN repository_configurations validated
        ON validated.org_id = rc.org_id
        AND validated.deleted_at IS NULL
        AND validated.name = CASE rc.name
            WHEN 'lightwell/java/remediated' THEN 'lightwell/java/validated'
            WHEN 'lightwell/python/remediated' THEN 'lightwell/python/validated'
        END
    CROSS JOIN LATERAL unnest(la.fixed_versions) AS ver
    WHERE rc.org_id = '-3'
        AND rc.deleted_at IS NULL
        AND rc.name IN ('lightwell/java/remediated', 'lightwell/python/remediated')
),
split_versions AS (
    SELECT
        remediated_uuid,
        validated_uuid,
        content_type,
        package_group,
        package_name,
        stored_version,
        CASE
            WHEN stored_version ~* '[.+-]rhlw' THEN substring(stored_version FROM '(?i)(rhlw.*)$')
            ELSE ''
        END AS release,
        CASE
            WHEN stored_version ~* '[.+-]rhlw' THEN regexp_replace(stored_version, '(?i)[.+-]rhlw.*$', '')
            ELSE stored_version
        END AS upstream_version,
        published
    FROM seed_advisories
    WHERE package_name <> ''
),
validated_rows AS (
    SELECT DISTINCT ON (validated_uuid, package_group, package_name, upstream_version)
        validated_uuid AS repo_uuid,
        content_type,
        package_group,
        package_name,
        upstream_version AS version,
        ''::text AS release,
        upstream_version,
        published
    FROM split_versions
    ORDER BY validated_uuid, package_group, package_name, upstream_version, published
)
SELECT
    remediated_uuid AS repo_uuid,
    content_type,
    package_group,
    package_name,
    stored_version AS version,
    release,
    upstream_version,
    published
FROM split_versions
UNION ALL
SELECT
    repo_uuid,
    content_type,
    package_group,
    package_name,
    version,
    release,
    upstream_version,
    published
FROM validated_rows;

INSERT INTO lightwell_packages (
    uuid,
    repository_configuration_uuid,
    name,
    package_group
)
SELECT DISTINCT
    (
        substr(pkg_key, 1, 8) || '-' ||
        substr(pkg_key, 9, 4) || '-4' ||
        substr(pkg_key, 14, 3) || '-8' ||
        substr(pkg_key, 18, 3) || '-' ||
        substr(pkg_key, 21, 12)
    )::uuid,
    repo_uuid,
    package_name,
    package_group
FROM (
    SELECT
        repo_uuid,
        package_group,
        package_name,
        md5('pkg|' || repo_uuid::text || '|' || package_group || '|' || package_name) AS pkg_key
    FROM seed_package_versions
) packages;

INSERT INTO lightwell_package_versions (
    uuid,
    lightwell_package_uuid,
    repository_configuration_uuid,
    version,
    release,
    published_at,
    purl,
    upstream_version,
    project_url,
    license,
    summary,
    description,
    author,
    author_email
)
SELECT
    (
        substr(ver_key, 1, 8) || '-' ||
        substr(ver_key, 9, 4) || '-4' ||
        substr(ver_key, 14, 3) || '-8' ||
        substr(ver_key, 18, 3) || '-' ||
        substr(ver_key, 21, 12)
    )::uuid,
    (
        substr(pkg_key, 1, 8) || '-' ||
        substr(pkg_key, 9, 4) || '-4' ||
        substr(pkg_key, 14, 3) || '-8' ||
        substr(pkg_key, 18, 3) || '-' ||
        substr(pkg_key, 21, 12)
    )::uuid,
    repo_uuid,
    version,
    release,
    to_char(published AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
    CASE
        WHEN content_type = 'maven' THEN 'pkg:maven/' || package_group || '/' || package_name || '@' || version
        ELSE 'pkg:pypi/' || package_name || '@' || version
    END,
    upstream_version,
    'https://example.com/' || package_name,
    'Apache-2.0',
    'Seed summary for ' || package_name,
    'Seed description for ' || package_name,
    CASE WHEN content_type = 'maven' THEN 'Example Org' ELSE 'Example Author' END,
    CASE WHEN content_type = 'maven' THEN '' ELSE 'author@example.com' END
FROM (
    SELECT
        repo_uuid,
        content_type,
        package_group,
        package_name,
        version,
        release,
        upstream_version,
        published,
        md5('pkg|' || repo_uuid::text || '|' || package_group || '|' || package_name) AS pkg_key,
        md5('ver|' || repo_uuid::text || '|' || package_group || '|' || package_name || '|' || version) AS ver_key
    FROM seed_package_versions
) versions;

COMMIT;
