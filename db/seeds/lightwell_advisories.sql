-- Dev seed for lightwell advisories.
-- Apply db/seeds/lightwell_vulnerabilities.sql first, and create the
-- Lightwell repositories with scripts/create_lightwell_repo.sh. This script
-- reads the vulnerability seed and inserts one advisory per vulnerability
-- whose stage is 'Lightwell Network'. package_name comes from the
-- vulnerability purl, package_version is component_version, and release rows
-- follow published_versions.
-- Java advisories belong to lightwell/java/remediated and Python advisories
-- to lightwell/python/remediated. JavaScript and C# stay on lightwell/seed.
BEGIN;

INSERT INTO repositories (
    uuid,
    url,
    public,
    origin,
    content_type,
    last_introspection_status
) VALUES (
    '00000000-0000-4000-8000-0000000000b0',
    'https://lightwell.example.com/seed',
    true,
    'lightwell',
    'maven',
    'Valid'
)
ON CONFLICT (uuid) DO NOTHING;

INSERT INTO repository_configurations (
    uuid,
    created_at,
    updated_at,
    name,
    label,
    arch,
    org_id,
    repository_uuid,
    feature_name
) VALUES (
    '00000000-0000-4000-8000-0000000000c0',
    TIMESTAMPTZ '2026-08-18 00:00:00+00',
    TIMESTAMPTZ '2026-08-18 00:00:00+00',
    'lightwell/seed',
    'lightwell_seed',
    'x86_64',
    '-3',
    '00000000-0000-4000-8000-0000000000b0',
    'lightwell-network'
)
ON CONFLICT (uuid) DO NOTHING;

DO $$
BEGIN
    IF (
        SELECT count(*) FROM repository_configurations
        WHERE org_id = '-3'
            AND deleted_at IS NULL
            AND name IN (
                'lightwell/java/validated',
                'lightwell/java/remediated',
                'lightwell/python/validated',
                'lightwell/python/remediated'
            )
    ) < 4 THEN
        RAISE EXCEPTION
            'Missing Lightwell repositories in org -3. Run scripts/create_lightwell_repo.sh first.';
    END IF;
END $$;

-- Drop the standalone seed repos from an earlier version of this script.
-- Advisories and packages on them cascade away and are recreated below.
DELETE FROM repository_configurations
WHERE uuid IN (
    '00000000-0000-4000-8000-0000000000c1'::uuid,
    '00000000-0000-4000-8000-0000000000c2'::uuid,
    '00000000-0000-4000-8000-0000000000c3'::uuid,
    '00000000-0000-4000-8000-0000000000c4'::uuid
);
DELETE FROM repositories
WHERE uuid IN (
    '00000000-0000-4000-8000-0000000000b1'::uuid,
    '00000000-0000-4000-8000-0000000000b2'::uuid,
    '00000000-0000-4000-8000-0000000000b3'::uuid,
    '00000000-0000-4000-8000-0000000000b4'::uuid
);

INSERT INTO lightwell_advisories (
    uuid,
    repo_name,
    advisory_id,
    severity,
    severity_score,
    details,
    reference_urls,
    package_name,
    package_version,
    fixed_versions,
    repository_configuration_uuid,
    checksum,
    published,
    modified,
    aliases,
    schema_version,
    source,
    summary
)
SELECT
    overlay(v.uuid::text placing 'a' from 15 for 1)::uuid,
    CASE v.language
        WHEN 'java' THEN 'lightwell/java/remediated'
        WHEN 'python' THEN 'lightwell/python/remediated'
        WHEN 'javascript' THEN 'lightwell/seed/npm'
        WHEN 'csharp' THEN 'lightwell/seed/nuget'
        ELSE 'lightwell/seed'
    END,
    v.vulnerability_id,
    COALESCE(v.cvss::text, ''),
    COALESCE(v.cvss, 0)::real,
    COALESCE(v.description, ''),
    ARRAY['https://lightwell.example.com/vulnerabilities/' || v.vulnerability_id],
    CASE
        WHEN v.purl LIKE 'pkg:maven/%' THEN replace(substring(v.purl from 'pkg:maven/([^@]+)'), '/', ':')
        WHEN v.purl LIKE 'pkg:pypi/%' THEN substring(v.purl from 'pkg:pypi/([^@]+)')
        WHEN v.purl LIKE 'pkg:npm/%' THEN substring(v.purl from 'pkg:npm/([^@]+)')
        WHEN v.purl LIKE 'pkg:nuget/%' THEN substring(v.purl from 'pkg:nuget/([^@]+)')
        ELSE v.component_name
    END,
    v.component_version,
    v.published_versions,
    CASE v.language
        WHEN 'java' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/java/remediated' AND rc.deleted_at IS NULL
        )
        WHEN 'python' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/python/remediated' AND rc.deleted_at IS NULL
        )
        ELSE '00000000-0000-4000-8000-0000000000c0'::uuid
    END,
    'seed-' || v.vulnerability_id,
    v.last_updated,
    v.last_updated,
    ARRAY[v.vulnerability_key],
    '1.6.8',
    'lightwell-seed',
    COALESCE(v.title, '')
FROM lightwell_vulnerabilities v
WHERE v.stage = 'Lightwell Network'
    AND v.uuid::text LIKE '00000000-0000-4000-8000-%'
ON CONFLICT (uuid) DO UPDATE SET
    repo_name = EXCLUDED.repo_name,
    repository_configuration_uuid = EXCLUDED.repository_configuration_uuid;

INSERT INTO lightwell_advisory_releases (
    advisory_uuid,
    release_version,
    rhlw_baseline,
    rhlw_novel,
    rhlw_hotfix
)
SELECT
    a.uuid,
    ver,
    COALESCE((regexp_match(ver, 'rhlw-(\d+)'))[1]::int, 0),
    0,
    0
FROM lightwell_advisories a
CROSS JOIN LATERAL unnest(a.fixed_versions) AS ver
WHERE a.uuid::text LIKE '00000000-0000-a000-8000-%'
ON CONFLICT (advisory_uuid, release_version) DO NOTHING;

COMMIT;
