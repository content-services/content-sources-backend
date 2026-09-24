-- name: ListAdvisories :many
WITH filtered AS (
    SELECT
        la.uuid,
        la.advisory_id,
        la.severity,
        la.severity_score,
        la.details,
        la.reference_urls,
        la.package_name,
        la.package_version,
        la.fixed_versions,
        la.repo_name,
        la.repository_configuration_uuid,
        la.published,
        la.modified,
        la.aliases,
        la.schema_version,
        la.source,
        la.summary,
        la.created_at,
        la.updated_at
    FROM lightwell_advisories la
    JOIN repository_configurations rc
      ON rc.uuid = la.repository_configuration_uuid
    WHERE 1=1
        AND (
            sqlc.narg(repository_config_uuid)::uuid IS NULL
            OR la.repository_configuration_uuid = sqlc.narg(repository_config_uuid)::uuid
        )
        AND (
            sqlc.narg(repo_name)::text IS NULL
            OR la.repo_name = sqlc.narg(repo_name)::text
        )
        AND (
            sqlc.narg(package_name)::text IS NULL
            OR la.package_name ILIKE '%' || sqlc.narg(package_name)::text || '%'
        )
        AND (
            sqlc.narg(severity_min)::real IS NULL
            OR la.severity_score >= sqlc.narg(severity_min)::real
        )
        AND (
            sqlc.narg(cve_id)::text IS NULL
            OR la.advisory_id = sqlc.narg(cve_id)::text
        )
        AND (
            sqlc.narg(name)::text IS NULL
            OR la.advisory_id ILIKE '%' || sqlc.narg(name)::text || '%'
            OR la.aliases::text ILIKE '%' || sqlc.narg(name)::text || '%'
        )
        AND (
            sqlc.narg(package_version)::text IS NULL
            OR la.advisory_id ILIKE '%' || sqlc.narg(package_version)::text || '%'
        )
        AND (
            sqlc.narg(entitled_features)::text[] IS NULL
            OR EXISTS (
                SELECT 1
                FROM unnest(string_to_array(rc.feature_name, ',')) AS t(token)
                WHERE btrim(t.token) = ANY(sqlc.narg(entitled_features)::text[])
            )
        )
),
max_rank AS (
    SELECT r.rhlw_baseline, r.rhlw_novel, r.rhlw_hotfix
    FROM filtered f
    INNER JOIN lightwell_advisory_releases r ON r.advisory_uuid = f.uuid
    ORDER BY r.rhlw_baseline DESC, r.rhlw_novel DESC, r.rhlw_hotfix DESC
    LIMIT 1
)
SELECT
    uuid,
    advisory_id,
    severity,
    severity_score,
    details,
    reference_urls,
    package_name,
    package_version,
    fixed_versions,
    repo_name,
    repository_configuration_uuid,
    published,
    modified,
    aliases,
    schema_version,
    source,
    summary,
    created_at,
    updated_at,
    COUNT(*) OVER() AS total_count
FROM filtered f
WHERE
    sqlc.narg(latest_release)::boolean IS NOT TRUE
    OR EXISTS (
        SELECT 1
        FROM lightwell_advisory_releases r
        INNER JOIN max_rank m
            ON r.rhlw_baseline = m.rhlw_baseline
            AND r.rhlw_novel = m.rhlw_novel
            AND r.rhlw_hotfix = m.rhlw_hotfix
        WHERE r.advisory_uuid = f.uuid
    )
ORDER BY severity_score DESC, created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAdvisoriesByRepo :one
SELECT COUNT(*)::bigint AS total
FROM lightwell_advisories la
WHERE la.repository_configuration_uuid = sqlc.arg(repository_config_uuid)::uuid;

-- name: ListAdvisoriesByPackage :many
SELECT
    la.advisory_id,
    la.severity,
    la.severity_score,
    la.details,
    la.fixed_versions,
    la.repo_name
FROM lightwell_advisories la
WHERE la.package_name = sqlc.arg(package_name)::text
ORDER BY la.severity_score DESC, la.created_at DESC;

-- name: ListAdvisoriesByCveID :many
SELECT
    la.package_name,
    la.fixed_versions,
    la.repo_name,
    la.severity
FROM lightwell_advisories la
WHERE la.advisory_id = sqlc.arg(cve_id)::text;
