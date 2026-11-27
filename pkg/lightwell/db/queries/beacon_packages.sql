-- name: ListBeaconPackages :one
WITH filtered AS MATERIALIZED (
    SELECT coalesce(v.language, '') AS ecosystem, v.component_name, v.component_version,
           v.vulnerability_id, v.severity, v.stage, v.last_updated, v.embargo, v.published_versions
    FROM lightwell_filtered_vulnerabilities(
        sqlc.arg(customer_id), sqlc.narg(severities)::text[], sqlc.narg(stages)::text[],
        sqlc.narg(complexities)::text[], sqlc.narg(ticket_ids)::text[],
        sqlc.narg(flags)::text[], sqlc.narg(search)::text
    ) v
), candidates AS (
    SELECT ecosystem, component_name, component_version, max(last_updated) AS latest_update
    FROM filtered
    GROUP BY ecosystem, component_name, component_version
), addressed AS MATERIALIZED (
    -- One pass over the customer's vulnerabilities instead of a correlated
    -- NOT EXISTS per candidate package. The all_addressed guard in the WHERE
    -- clause is a constant, so the scan is skipped entirely when the filter is
    -- off.
    SELECT coalesce(v.language, '') AS ecosystem, v.component_name, v.component_version,
           bool_and(v.stage IS NOT DISTINCT FROM 'Lightwell Network') AS all_addressed
    FROM lightwell_vulnerability_customers vc
    JOIN lightwell_vulnerabilities v ON v.uuid = vc.vulnerability_uuid
    WHERE sqlc.arg(all_addressed)::boolean
      AND vc.customer_id = sqlc.arg(customer_id)
    GROUP BY 1, 2, 3
), eligible AS MATERIALIZED (
    SELECT p.*
    FROM candidates p
    LEFT JOIN addressed a USING (ecosystem, component_name, component_version)
    WHERE NOT sqlc.arg(all_addressed)::boolean OR coalesce(a.all_addressed, true)
), page AS MATERIALIZED (
    SELECT *
    FROM eligible
    ORDER BY latest_update DESC, ecosystem, component_name, component_version
    LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset)
), page_records AS MATERIALIZED (
    SELECT f.*
    FROM filtered f
    JOIN page p USING (ecosystem, component_name, component_version)
), severities AS (
    SELECT ecosystem, component_name, component_version, vulnerability_id,
           max(array_position(sqlc.arg(severity_rank)::text[], severity)) AS rank
    FROM page_records
    GROUP BY ecosystem, component_name, component_version, vulnerability_id
), severity_counts AS (
    SELECT ecosystem, component_name, component_version, count(*) AS vulnerability_count,
           (sqlc.arg(severity_rank)::text[])[max(rank)] AS highest_severity,
           -- rank is the 1-based position in the severity_rank argument, which is always
           -- dao.lightwellSeverityRank = {Low, Moderate, Important, Critical}. These literals
           -- must be kept in step with that slice; see the comment on it before changing either.
           jsonb_build_object(
               'critical', count(*) FILTER (WHERE rank = 4),
               'important', count(*) FILTER (WHERE rank = 3),
               'moderate', count(*) FILTER (WHERE rank = 2),
               'low', count(*) FILTER (WHERE rank = 1)
           ) AS severity_counts
    FROM severities
    GROUP BY ecosystem, component_name, component_version
), statuses AS (
    SELECT ecosystem, component_name, component_version, stage, count(DISTINCT vulnerability_id) AS count
    FROM page_records
    GROUP BY ecosystem, component_name, component_version, stage
), status_counts AS (
    SELECT ecosystem, component_name, component_version, jsonb_object_agg(stage, count) AS status_counts
    FROM statuses
    GROUP BY ecosystem, component_name, component_version
), versions AS (
    SELECT ecosystem, component_name, component_version, array_agg(DISTINCT version ORDER BY version) AS published_versions
    FROM page_records
    CROSS JOIN LATERAL unnest(published_versions) AS version
    GROUP BY ecosystem, component_name, component_version
), eligible_records AS MATERIALIZED (
    -- Summary totals cover every eligible package, not just the page, but must honour
    -- all_addressed the same way the list does -- otherwise the header contradicts the rows.
    SELECT f.*
    FROM filtered f
    JOIN eligible e USING (ecosystem, component_name, component_version)
), summary_vulnerabilities AS (
    -- One row per package/vulnerability, mirroring the per-package `severities` CTE, so the
    -- header totals are exactly the sum of the per-package vulnerability_count values rather
    -- than a record count that silently exceeds them.
    SELECT (sqlc.arg(severity_rank)::text[])[max(array_position(sqlc.arg(severity_rank)::text[], severity))] AS severity,
           bool_or(embargo) AS embargo
    FROM eligible_records
    GROUP BY ecosystem, component_name, component_version, vulnerability_id
), summary_statuses AS (
    SELECT stage, count(*) AS count
    FROM (SELECT DISTINCT ecosystem, component_name, component_version, vulnerability_id, stage FROM eligible_records) s
    GROUP BY stage
)
SELECT
    (SELECT count(*) FROM eligible) AS package_count,
    (SELECT count(*) FROM summary_vulnerabilities) AS vulnerability_count,
    (SELECT count(*) FROM summary_vulnerabilities WHERE severity = 'Critical') AS critical_count,
    (SELECT count(*) FROM summary_vulnerabilities WHERE embargo) AS embargo_count,
    coalesce((SELECT jsonb_object_agg(stage, count) FROM summary_statuses), '{}'::jsonb)::jsonb AS status_counts,
    coalesce((
        SELECT jsonb_agg(
            jsonb_build_object(
                'ecosystem', p.ecosystem, 'component_name', p.component_name, 'component_version', p.component_version,
                'vulnerability_count', s.vulnerability_count, 'highest_severity', s.highest_severity,
                'severity_counts', s.severity_counts, 'status_counts', t.status_counts,
                'published_versions', v.published_versions
            ) ORDER BY p.latest_update DESC, p.ecosystem, p.component_name, p.component_version
        )
        FROM page p
        JOIN severity_counts s USING (ecosystem, component_name, component_version)
        JOIN status_counts t USING (ecosystem, component_name, component_version)
        LEFT JOIN versions v USING (ecosystem, component_name, component_version)
    ), '[]'::jsonb)::jsonb AS packages;

-- name: ListBeaconPackageVulnerabilities :one
WITH matching AS MATERIALIZED (
    SELECT coalesce(array_position(ARRAY[
               'Submitted', 'Classified', 'Fix in Progress', 'Validation', 'Lightwell Network',
               'Upstreaming', 'Published', 'On Hold', 'Unremediated'
           ]::text[], v.stage), 0) AS pipeline_position,
           v.uuid, v.vulnerability_id, v.title, v.severity, v.cvss, v.stage AS status,
           v.last_updated, v.resolution_reason, coalesce(v.published_versions, '{}'::text[]) AS published_versions
    FROM lightwell_filtered_vulnerabilities(
        sqlc.arg(customer_id), NULL, NULL, NULL, NULL, NULL, NULL
    ) v
    WHERE coalesce(v.language, '') = sqlc.arg(ecosystem)::text
      AND v.component_name = sqlc.arg(component_name)::text
      AND v.component_version = sqlc.arg(component_version)::text
), page AS (
    SELECT * FROM matching
    ORDER BY pipeline_position, last_updated DESC, vulnerability_id, uuid
    LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset)
), statuses AS (
    SELECT status, count(DISTINCT vulnerability_id) AS count FROM matching GROUP BY status
)
SELECT
    (SELECT count(*) FROM matching) AS record_count,
    (SELECT count(DISTINCT vulnerability_id) FROM matching) AS vulnerability_count,
    coalesce((SELECT (sqlc.arg(severity_rank)::text[])[max(array_position(sqlc.arg(severity_rank)::text[], severity))] FROM matching), '')::text AS highest_severity,
    ARRAY(SELECT DISTINCT version FROM matching CROSS JOIN LATERAL unnest(published_versions) AS version ORDER BY version)::text[] AS published_versions,
    coalesce((SELECT jsonb_object_agg(status, count) FROM statuses), '{}'::jsonb)::jsonb AS status_counts,
    coalesce((SELECT jsonb_agg(to_jsonb(page) - 'pipeline_position' ORDER BY pipeline_position, last_updated DESC, vulnerability_id, uuid) FROM page), '[]'::jsonb)::jsonb AS vulnerabilities;
