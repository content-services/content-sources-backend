-- name: ListLightwellPackages :many
SELECT
    p.uuid,
    p.repository_configuration_uuid,
    rc.name AS repository_name,
    r.content_type AS ecosystem,
    p.name,
    p.package_group,
    array_agg(pv.version ORDER BY pv.version) AS versions,
    array_agg(pv.release ORDER BY pv.version) AS releases,
    array_agg(pv.published_at ORDER BY pv.version) AS published_ats,
    COUNT(*) OVER() AS total_count
FROM lightwell_packages p
JOIN repository_configurations rc ON rc.uuid = p.repository_configuration_uuid
JOIN repositories r ON r.uuid = rc.repository_uuid
LEFT JOIN lightwell_package_versions pv ON pv.lightwell_package_uuid = p.uuid
WHERE r.origin = 'lightwell'
    AND (sqlc.narg(ecosystem)::text IS NULL OR r.content_type = sqlc.narg(ecosystem)::text)
    AND (sqlc.narg(name)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(name)::text || '%')
    AND (sqlc.narg(repository)::text IS NULL OR lower(rc.name) = lower(sqlc.narg(repository)::text))
    AND (sqlc.narg(security_level)::text IS NULL OR lower(r.security_level) = lower(sqlc.narg(security_level)::text))
    AND (
        sqlc.narg(entitled_features)::text[] IS NULL
        OR rc.feature_name IS NULL
        OR btrim(rc.feature_name) = ''
        OR EXISTS (
            SELECT 1
            FROM unnest(string_to_array(rc.feature_name, ',')) AS t(token)
            WHERE btrim(t.token) = ANY(sqlc.narg(entitled_features)::text[])
        )
    )
GROUP BY p.uuid, p.repository_configuration_uuid, rc.name, r.content_type, p.name, p.package_group
ORDER BY p.package_group, p.name
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListLightwellPackageVersions :many
SELECT
    pv.uuid,
    pv.repository_configuration_uuid,
    rc.name AS repository_name,
    r.content_type AS ecosystem,
    p.name,
    p.package_group,
    pv.version,
    pv.release,
    pv.published_at,
    pv.purl,
    COUNT(*) OVER() AS total_count
FROM lightwell_package_versions pv
JOIN lightwell_packages p ON p.uuid = pv.lightwell_package_uuid
JOIN repository_configurations rc ON rc.uuid = pv.repository_configuration_uuid
JOIN repositories r ON r.uuid = rc.repository_uuid
WHERE r.origin = 'lightwell'
    AND (sqlc.narg(ecosystem)::text IS NULL OR r.content_type = sqlc.narg(ecosystem)::text)
    AND (sqlc.narg(name)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(name)::text || '%')
    AND (sqlc.narg(repository)::text IS NULL OR lower(rc.name) = lower(sqlc.narg(repository)::text))
    AND (sqlc.narg(security_level)::text IS NULL OR lower(r.security_level) = lower(sqlc.narg(security_level)::text))
    AND (
        sqlc.narg(entitled_features)::text[] IS NULL
        OR rc.feature_name IS NULL
        OR btrim(rc.feature_name) = ''
        OR EXISTS (
            SELECT 1
            FROM unnest(string_to_array(rc.feature_name, ',')) AS t(token)
            WHERE btrim(t.token) = ANY(sqlc.narg(entitled_features)::text[])
        )
    )
    AND (
        sqlc.narg(resolves_cve_id)::text IS NULL
        OR EXISTS (
            SELECT 1 FROM lightwell_advisories la
            WHERE la.repository_configuration_uuid = rc.uuid
                AND la.package_name = p.name
                AND la.advisory_id = sqlc.narg(resolves_cve_id)::text
                AND pv.version = ANY(la.fixed_versions)
        )
    )
    AND (
        sqlc.narg(vulnerable_to_cve_id)::text IS NULL
        OR (
            EXISTS (
                SELECT 1 FROM lightwell_advisories la
                WHERE la.repository_configuration_uuid = rc.uuid
                    AND la.package_name = p.name
                    AND la.advisory_id = sqlc.narg(vulnerable_to_cve_id)::text
            )
            AND NOT EXISTS (
                SELECT 1 FROM lightwell_advisories la
                WHERE la.repository_configuration_uuid = rc.uuid
                    AND la.package_name = p.name
                    AND la.advisory_id = sqlc.narg(vulnerable_to_cve_id)::text
                    AND pv.version = ANY(la.fixed_versions)
            )
        )
    )
ORDER BY p.package_group, p.name, pv.version
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
