-- name: ListCustomerIds :many
SELECT DISTINCT customer_id
FROM lightwell_vulnerability_customers
ORDER BY customer_id;

-- name: ListVulnerabilities :many
SELECT
    v.uuid,
    v.vulnerability_id,
    v.purl,
    v.component_name,
    v.component_version,
    v.title,
    v.cwe,
    v.description,
    v.severity,
    v.cvss,
    v.cvss_vector,
    v.exploit_tested,
    v.reproducer_included,
    v.customer_priority,
    v.stage,
    v.language,
    v.complexity,
    v.submitted_date,
    v.last_updated,
    v.embargo,
    v.duplicate,
    v.ltwwlsupt_ticket_id,
    v.created_at,
    v.updated_at
FROM lightwell_vulnerabilities v
INNER JOIN lightwell_vulnerability_customers vc ON vc.vulnerability_uuid = v.uuid
WHERE vc.customer_id = sqlc.arg(customer_id)
    AND (
        sqlc.narg(severities)::text[] IS NULL
        OR cardinality(sqlc.narg(severities)::text[]) = 0
        OR v.severity = ANY (sqlc.narg(severities)::text[])
    )
    AND (
        sqlc.narg(stages)::text[] IS NULL
        OR cardinality(sqlc.narg(stages)::text[]) = 0
        OR v.stage = ANY (sqlc.narg(stages)::text[])
    )
    AND (
        sqlc.narg(complexities)::text[] IS NULL
        OR cardinality(sqlc.narg(complexities)::text[]) = 0
        OR v.complexity = ANY (sqlc.narg(complexities)::text[])
    )
    AND (
        sqlc.narg(ltwwlsupt_ticket_ids)::text[] IS NULL
        OR cardinality(sqlc.narg(ltwwlsupt_ticket_ids)::text[]) = 0
        OR v.ltwwlsupt_ticket_id = ANY (sqlc.narg(ltwwlsupt_ticket_ids)::text[])
    )
    AND (
        sqlc.narg(flag)::text IS NULL
        OR (
            sqlc.narg(flag)::text = 'embargo'
            AND v.embargo = true
        )
        OR (
            sqlc.narg(flag)::text = 'duplicate'
            AND v.duplicate = true
        )
    )
    AND (
        sqlc.narg(search)::text IS NULL
        OR v.vulnerability_id ILIKE '%' || sqlc.narg(search) || '%'
        OR v.component_name ILIKE '%' || sqlc.narg(search) || '%'
        OR v.title ILIKE '%' || sqlc.narg(search) || '%'
    )
ORDER BY v.last_updated DESC, v.vulnerability_id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAggregates :one
SELECT
    COUNT(*)::bigint AS total_count,
    COUNT(*) FILTER (WHERE v.severity = 'Critical')::bigint AS critical_count,
    COUNT(*) FILTER (WHERE v.embargo = true)::bigint AS embargo_count,
    COUNT(*) FILTER (
        WHERE v.stage <> 'Lightwell Network'
            AND (CURRENT_DATE - v.submitted_date) > 30
    )::bigint AS blocked_count
FROM lightwell_vulnerabilities v
INNER JOIN lightwell_vulnerability_customers vc ON vc.vulnerability_uuid = v.uuid
WHERE vc.customer_id = sqlc.arg(customer_id)
    AND (
        sqlc.narg(severities)::text[] IS NULL
        OR cardinality(sqlc.narg(severities)::text[]) = 0
        OR v.severity = ANY (sqlc.narg(severities)::text[])
    )
    AND (
        sqlc.narg(stages)::text[] IS NULL
        OR cardinality(sqlc.narg(stages)::text[]) = 0
        OR v.stage = ANY (sqlc.narg(stages)::text[])
    )
    AND (
        sqlc.narg(complexities)::text[] IS NULL
        OR cardinality(sqlc.narg(complexities)::text[]) = 0
        OR v.complexity = ANY (sqlc.narg(complexities)::text[])
    )
    AND (
        sqlc.narg(ltwwlsupt_ticket_ids)::text[] IS NULL
        OR cardinality(sqlc.narg(ltwwlsupt_ticket_ids)::text[]) = 0
        OR v.ltwwlsupt_ticket_id = ANY (sqlc.narg(ltwwlsupt_ticket_ids)::text[])
    )
    AND (
        sqlc.narg(flag)::text IS NULL
        OR (
            sqlc.narg(flag)::text = 'embargo'
            AND v.embargo = true
        )
        OR (
            sqlc.narg(flag)::text = 'duplicate'
            AND v.duplicate = true
        )
    )
    AND (
        sqlc.narg(search)::text IS NULL
        OR v.vulnerability_id ILIKE '%' || sqlc.narg(search) || '%'
        OR v.component_name ILIKE '%' || sqlc.narg(search) || '%'
        OR v.title ILIKE '%' || sqlc.narg(search) || '%'
    );

-- name: CountByStage :many
SELECT
    v.stage,
    COUNT(*)::bigint AS count
FROM lightwell_vulnerabilities v
INNER JOIN lightwell_vulnerability_customers vc ON vc.vulnerability_uuid = v.uuid
WHERE vc.customer_id = sqlc.arg(customer_id)
    AND (
        sqlc.narg(severities)::text[] IS NULL
        OR cardinality(sqlc.narg(severities)::text[]) = 0
        OR v.severity = ANY (sqlc.narg(severities)::text[])
    )
    AND (
        sqlc.narg(stages)::text[] IS NULL
        OR cardinality(sqlc.narg(stages)::text[]) = 0
        OR v.stage = ANY (sqlc.narg(stages)::text[])
    )
    AND (
        sqlc.narg(complexities)::text[] IS NULL
        OR cardinality(sqlc.narg(complexities)::text[]) = 0
        OR v.complexity = ANY (sqlc.narg(complexities)::text[])
    )
    AND (
        sqlc.narg(ltwwlsupt_ticket_ids)::text[] IS NULL
        OR cardinality(sqlc.narg(ltwwlsupt_ticket_ids)::text[]) = 0
        OR v.ltwwlsupt_ticket_id = ANY (sqlc.narg(ltwwlsupt_ticket_ids)::text[])
    )
    AND (
        sqlc.narg(flag)::text IS NULL
        OR (
            sqlc.narg(flag)::text = 'embargo'
            AND v.embargo = true
        )
        OR (
            sqlc.narg(flag)::text = 'duplicate'
            AND v.duplicate = true
        )
    )
    AND (
        sqlc.narg(search)::text IS NULL
        OR v.vulnerability_id ILIKE '%' || sqlc.narg(search) || '%'
        OR v.component_name ILIKE '%' || sqlc.narg(search) || '%'
        OR v.title ILIKE '%' || sqlc.narg(search) || '%'
    )
GROUP BY v.stage
ORDER BY v.stage;
