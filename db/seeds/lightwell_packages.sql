-- Dev seed for Lightwell package coordinates (and a temporary mirror fill).
-- Apply db/seeds/lightwell_advisories.sql first. create_lightwell_repo.sh uses
-- these rows to seed Pulp, then import-lightwell-packages replaces the mirror.
--
-- Remediated versions are the advisory fixed_versions strings, including rhlw,
-- excluding predisclosure seed IDs (LWL-2026-36xx).
-- Validated versions are catalog upstream builds (see validated_catalog below).
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
            'lightwell/java/predisclosure',
            'lightwell/python/validated',
            'lightwell/python/remediated',
            'lightwell/python/predisclosure'
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
            'lightwell/java/predisclosure',
            'lightwell/python/validated',
            'lightwell/python/remediated',
            'lightwell/python/predisclosure'
        )
);

CREATE TEMP TABLE seed_package_versions ON COMMIT DROP AS
WITH seed_advisories AS (
    SELECT
        la.repository_configuration_uuid AS repo_uuid,
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
    CROSS JOIN LATERAL unnest(la.fixed_versions) AS ver
    WHERE rc.org_id = '-3'
        AND rc.deleted_at IS NULL
        AND rc.name IN (
            'lightwell/java/remediated',
            'lightwell/python/remediated',
            'lightwell/java/predisclosure',
            'lightwell/python/predisclosure'
        )
),
split_versions AS (
    SELECT
        repo_uuid,
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
validated_catalog AS (
    SELECT *
    FROM (
        VALUES
            ('maven'::text, 'com.squareup.okhttp3', 'okhttp', '4.12.0'),
            ('maven', 'net.minidev', 'json-smart', '2.5.0'),
            ('maven', 'org.apache.poi', 'poi', '5.2.4'),
            ('maven', 'org.apache.shiro', 'shiro-core', '1.11.0'),
            ('maven', 'org.codehaus.plexus', 'plexus-utils', '3.5.0'),
            ('maven', 'org.eclipse.jetty', 'jetty-server', '11.0.14'),
            ('maven', 'org.hibernate', 'hibernate-core', '5.6.15.Final'),
            ('maven', 'org.postgresql', 'postgresql', '42.5.2'),
            ('python', '', 'certifi', '2023.1.10'),
            ('python', '', 'jinja2', '3.1.3'),
            ('python', '', 'lxml', '4.9.3'),
            ('python', '', 'pyyaml', '6.0.1'),
            ('python', '', 'setuptools', '67.1.0'),
            ('python', '', 'werkzeug', '2.2.4')
    ) AS v(content_type, package_group, package_name, version)
),
validated_rows AS (
    SELECT
        rc.uuid AS repo_uuid,
        vc.content_type,
        vc.package_group,
        vc.package_name,
        vc.version,
        ''::text AS release,
        vc.version AS upstream_version,
        TIMESTAMPTZ '2026-08-18 00:00:00+00' AS published
    FROM validated_catalog vc
    JOIN repository_configurations rc
        ON rc.org_id = '-3'
        AND rc.deleted_at IS NULL
        AND rc.name = CASE vc.content_type
            WHEN 'maven' THEN 'lightwell/java/validated'
            WHEN 'python' THEN 'lightwell/python/validated'
        END
)
SELECT
    repo_uuid,
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
