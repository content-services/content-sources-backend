-- name: ListAdvisories :many
SELECT
    la.uuid,
    la.advisory_id,
    la.severity,
    la.severity_order,
    la.details,
    la.reference_urls,
    la.package_name,
    la.fixed_versions,
    la.repo_name,
    la.repository_configuration_uuid,
    la.created_at,
    COUNT(*) OVER() AS total_count
FROM lightwell_advisories la
WHERE 1=1
    AND (
        sqlc.narg(repository_config_uuid)::uuid IS NULL
        OR la.repository_configuration_uuid = sqlc.narg(repository_config_uuid)::uuid
    )
    AND (
        sqlc.narg(package_name)::text IS NULL
        OR la.package_name ILIKE '%' || sqlc.narg(package_name)::text || '%'
    )
    AND (
        sqlc.narg(severity_min)::smallint IS NULL
        OR la.severity_order >= sqlc.narg(severity_min)::smallint
    )
    AND (
        sqlc.narg(cve_id)::text IS NULL
        OR la.advisory_id = sqlc.narg(cve_id)::text
    )
ORDER BY la.severity_order DESC, la.created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAdvisoriesByRepo :one
SELECT COUNT(*)::bigint AS total
FROM lightwell_advisories la
WHERE la.repository_configuration_uuid = sqlc.arg(repository_config_uuid)::uuid;

-- name: ListAdvisoriesByPackage :many
SELECT
    la.advisory_id,
    la.severity,
    la.severity_order,
    la.details,
    la.fixed_versions,
    la.repo_name
FROM lightwell_advisories la
WHERE la.package_name = sqlc.arg(package_name)::text
ORDER BY la.severity_order DESC, la.created_at DESC;

-- name: ListAdvisoriesByCveID :many
SELECT
    la.package_name,
    la.fixed_versions,
    la.repo_name,
    la.severity
FROM lightwell_advisories la
WHERE la.advisory_id = sqlc.arg(cve_id)::text;
