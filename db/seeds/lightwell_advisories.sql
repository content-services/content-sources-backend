-- Dev seed for lightwell advisories.
-- Apply db/seeds/lightwell_vulnerabilities.sql first, and create the
-- Lightwell repositories with scripts/create_lightwell_repo.sh. This script
-- reads the vulnerability seed and inserts one advisory per vulnerability
-- whose stage is 'Lightwell Network'. package_name comes from the
-- vulnerability purl, package_version is component_version, and release rows
-- follow published_versions.
-- Java advisories belong to lightwell/java/remediated or lightwell/java/predisclosure,
-- Python advisories to lightwell/python/remediated or lightwell/python/predisclosure,
-- and JavaScript advisories to lightwell/javascript/remediated.
BEGIN;

DO $$
BEGIN
    IF (
        SELECT count(*) FROM repository_configurations
        WHERE org_id = '-3'
            AND deleted_at IS NULL
            AND name IN (
                'lightwell/java/validated',
                'lightwell/java/remediated',
                'lightwell/java/predisclosure',
                'lightwell/python/validated',
                'lightwell/python/remediated',
                'lightwell/python/predisclosure',
                'lightwell/javascript/validated',
                'lightwell/javascript/remediated'
            )
    ) < 8 THEN
        RAISE EXCEPTION
            'Missing Lightwell repositories in org -3. Run scripts/create_lightwell_repo.sh first.';
    END IF;
END $$;

-- Drop placeholder seed repositories, including lightwell/seed.
-- Advisories attached to them cascade away.
DELETE FROM repository_configurations
WHERE uuid IN (
    '00000000-0000-4000-8000-0000000000c0'::uuid,
    '00000000-0000-4000-8000-0000000000c1'::uuid,
    '00000000-0000-4000-8000-0000000000c2'::uuid,
    '00000000-0000-4000-8000-0000000000c3'::uuid,
    '00000000-0000-4000-8000-0000000000c4'::uuid
);
DELETE FROM repositories
WHERE uuid IN (
    '00000000-0000-4000-8000-0000000000b0'::uuid,
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
    upstream,
    schema_version,
    summary
)
SELECT
    overlay(v.uuid::text placing 'a' from 15 for 1)::uuid,
    CASE
        WHEN v.vulnerability_id LIKE 'LWL-2026-36%' AND v.language = 'java' THEN 'lightwell/java/predisclosure'
        WHEN v.vulnerability_id LIKE 'LWL-2026-36%' AND v.language = 'python' THEN 'lightwell/python/predisclosure'
        WHEN v.language = 'java' THEN 'lightwell/java/remediated'
        WHEN v.language = 'python' THEN 'lightwell/python/remediated'
        WHEN v.language = 'javascript' THEN 'lightwell/javascript/remediated'
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
        ELSE v.component_name
    END,
    v.component_version,
    v.published_versions,
    CASE
        WHEN v.vulnerability_id LIKE 'LWL-2026-36%' AND v.language = 'java' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/java/predisclosure' AND rc.deleted_at IS NULL
        )
        WHEN v.vulnerability_id LIKE 'LWL-2026-36%' AND v.language = 'python' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/python/predisclosure' AND rc.deleted_at IS NULL
        )
        WHEN v.language = 'java' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/java/remediated' AND rc.deleted_at IS NULL
        )
        WHEN v.language = 'python' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/python/remediated' AND rc.deleted_at IS NULL
        )
        WHEN v.language = 'javascript' THEN (
            SELECT rc.uuid FROM repository_configurations rc
            WHERE rc.org_id = '-3' AND rc.name = 'lightwell/javascript/remediated' AND rc.deleted_at IS NULL
        )
    END,
    'seed-' || v.vulnerability_id,
    v.last_updated,
    v.last_updated,
    seed_ids.ids,
    seed_ids.ids,
    '1.9.0',
    COALESCE(v.title, '')
FROM lightwell_vulnerabilities v
CROSS JOIN LATERAL (
    SELECT ARRAY_REMOVE(ARRAY[
        CASE
            WHEN substring(v.vulnerability_id from '^LWL-2026-([0-9]+)$') IS NOT NULL
            THEN 'CVE-2026-' || substring(v.vulnerability_id from '^LWL-2026-([0-9]+)$')
        END,
        v.vulnerability_key
    ], NULL) AS ids
) seed_ids
WHERE v.stage = 'Lightwell Network'
    AND v.language IN ('java', 'python', 'javascript')
    AND v.uuid::text LIKE '00000000-0000-4000-8000-%'
ON CONFLICT (uuid) DO UPDATE SET
    repo_name = EXCLUDED.repo_name,
    repository_configuration_uuid = EXCLUDED.repository_configuration_uuid,
    aliases = EXCLUDED.aliases,
    upstream = EXCLUDED.upstream,
    schema_version = EXCLUDED.schema_version;

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
    COALESCE((regexp_match(ver, 'rhlw[.+-](\d+)'))[1]::int, 0),
    0,
    0
FROM lightwell_advisories a
CROSS JOIN LATERAL unnest(a.fixed_versions) AS ver
WHERE a.uuid::text LIKE '00000000-0000-a000-8000-%'
ON CONFLICT (advisory_uuid, release_version) DO NOTHING;

COMMIT;
