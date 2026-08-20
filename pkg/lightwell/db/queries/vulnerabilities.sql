-- name: ListCustomerIds :many
SELECT DISTINCT customer_id
FROM lightwell_vulnerability_customers
ORDER BY customer_id;

-- name: ListLtwlsuptTicketIds :many
SELECT DISTINCT ticket_id
FROM lightwell_vulnerability_support_tickets
WHERE customer_id = sqlc.arg(customer_id)
ORDER BY ticket_id;

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
    v.duplicate_of,
    ARRAY(
        SELECT t.ticket_id
        FROM lightwell_vulnerability_support_tickets t
        WHERE t.vulnerability_uuid = v.uuid
            AND t.customer_id = sqlc.arg(customer_id)
        ORDER BY t.ticket_id
    )::text[] AS ltwlsupt_ticket_ids,
    v.created_at,
    v.updated_at
FROM lightwell_filtered_vulnerabilities(
    sqlc.arg(customer_id),
    sqlc.narg(severities)::text[],
    sqlc.narg(stages)::text[],
    sqlc.narg(complexities)::text[],
    sqlc.narg(ltwlsupt_ticket_ids)::text[],
    sqlc.narg(flags)::text[],
    sqlc.narg(search)::text
) AS v
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
FROM lightwell_filtered_vulnerabilities(
    sqlc.arg(customer_id),
    sqlc.narg(severities)::text[],
    sqlc.narg(stages)::text[],
    sqlc.narg(complexities)::text[],
    sqlc.narg(ltwlsupt_ticket_ids)::text[],
    sqlc.narg(flags)::text[],
    sqlc.narg(search)::text
) AS v;

-- name: CountByStage :many
SELECT
    v.stage,
    COUNT(*)::bigint AS count
FROM lightwell_filtered_vulnerabilities(
    sqlc.arg(customer_id),
    sqlc.narg(severities)::text[],
    sqlc.narg(stages)::text[],
    sqlc.narg(complexities)::text[],
    sqlc.narg(ltwlsupt_ticket_ids)::text[],
    sqlc.narg(flags)::text[],
    sqlc.narg(search)::text
) AS v
GROUP BY v.stage
ORDER BY v.stage;
