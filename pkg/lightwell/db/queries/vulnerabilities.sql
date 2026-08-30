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

-- name: GetVulnerabilityByKey :one
SELECT *
FROM lightwell_vulnerabilities
WHERE vulnerability_key = sqlc.arg(vulnerability_key);

-- name: DeleteVulnerabilityByKey :execrows
DELETE FROM lightwell_vulnerabilities
WHERE vulnerability_key = sqlc.arg(vulnerability_key);

-- name: UpsertVulnerability :one
INSERT INTO lightwell_vulnerabilities (
    uuid, vulnerability_key, vulnerability_id, purl, component_name, component_version, title, cwe, description,
    severity, cvss, cvss_vector, exploit_tested, reproducer_included, customer_priority, stage,
    language, complexity, submitted_date, last_updated, embargo, duplicate
) VALUES (
    sqlc.arg(uuid), sqlc.arg(vulnerability_key), sqlc.arg(vulnerability_id), sqlc.narg(purl), sqlc.arg(component_name),
    sqlc.arg(component_version), sqlc.narg(title), sqlc.narg(cwe), sqlc.narg(description),
    sqlc.arg(severity), sqlc.narg(cvss), sqlc.narg(cvss_vector), sqlc.arg(exploit_tested),
    sqlc.arg(reproducer_included), sqlc.narg(customer_priority), sqlc.arg(stage),
    sqlc.narg(language), sqlc.arg(complexity), sqlc.arg(submitted_date), sqlc.arg(last_updated),
    sqlc.arg(embargo), sqlc.arg(duplicate)
)
ON CONFLICT (vulnerability_key) DO UPDATE SET
    vulnerability_id = EXCLUDED.vulnerability_id,
    purl = EXCLUDED.purl,
    component_name = EXCLUDED.component_name,
    component_version = EXCLUDED.component_version,
    title = EXCLUDED.title,
    cwe = EXCLUDED.cwe,
    description = EXCLUDED.description,
    severity = EXCLUDED.severity,
    cvss = EXCLUDED.cvss,
    cvss_vector = EXCLUDED.cvss_vector,
    exploit_tested = EXCLUDED.exploit_tested,
    reproducer_included = EXCLUDED.reproducer_included,
    customer_priority = EXCLUDED.customer_priority,
    stage = CASE
        WHEN lightwell_vulnerabilities.stage = 'Lightwell Network' AND EXCLUDED.stage = 'Validation'
            THEN lightwell_vulnerabilities.stage
        ELSE EXCLUDED.stage
    END,
    language = EXCLUDED.language,
    complexity = EXCLUDED.complexity,
    submitted_date = EXCLUDED.submitted_date,
    last_updated = EXCLUDED.last_updated,
    embargo = EXCLUDED.embargo,
    duplicate = EXCLUDED.duplicate,
    updated_at = NOW()
WHERE (
    lightwell_vulnerabilities.vulnerability_id, lightwell_vulnerabilities.purl,
    lightwell_vulnerabilities.component_name, lightwell_vulnerabilities.component_version,
    lightwell_vulnerabilities.title, lightwell_vulnerabilities.cwe,
    lightwell_vulnerabilities.description, lightwell_vulnerabilities.severity,
    lightwell_vulnerabilities.cvss, lightwell_vulnerabilities.cvss_vector,
    lightwell_vulnerabilities.exploit_tested, lightwell_vulnerabilities.reproducer_included,
    lightwell_vulnerabilities.customer_priority, lightwell_vulnerabilities.stage,
    lightwell_vulnerabilities.language, lightwell_vulnerabilities.complexity,
    lightwell_vulnerabilities.submitted_date, lightwell_vulnerabilities.last_updated,
    lightwell_vulnerabilities.embargo, lightwell_vulnerabilities.duplicate
) IS DISTINCT FROM (
    EXCLUDED.vulnerability_id, EXCLUDED.purl, EXCLUDED.component_name,
    EXCLUDED.component_version, EXCLUDED.title, EXCLUDED.cwe, EXCLUDED.description,
    EXCLUDED.severity, EXCLUDED.cvss, EXCLUDED.cvss_vector, EXCLUDED.exploit_tested,
    EXCLUDED.reproducer_included, EXCLUDED.customer_priority,
    CASE
        WHEN lightwell_vulnerabilities.stage = 'Lightwell Network' AND EXCLUDED.stage = 'Validation'
            THEN lightwell_vulnerabilities.stage
        ELSE EXCLUDED.stage
    END,
    EXCLUDED.language, EXCLUDED.complexity, EXCLUDED.submitted_date, EXCLUDED.last_updated,
    EXCLUDED.embargo, EXCLUDED.duplicate
)
RETURNING uuid, (xmax = 0) AS inserted;

-- name: InsertVulnerabilityCustomer :exec
INSERT INTO lightwell_vulnerability_customers (customer_id, vulnerability_uuid)
VALUES (sqlc.arg(customer_id), sqlc.arg(vulnerability_uuid))
ON CONFLICT DO NOTHING;

-- name: UpsertVulnerabilityTicket :exec
INSERT INTO lightwell_vulnerability_support_tickets (vulnerability_uuid, customer_id, ticket_id)
VALUES (sqlc.arg(vulnerability_uuid), sqlc.arg(customer_id), sqlc.arg(ticket_id))
ON CONFLICT (vulnerability_uuid, ticket_id) DO UPDATE SET
    customer_id = EXCLUDED.customer_id;

-- name: DeleteVulnerabilityTicketsNotIn :exec
DELETE FROM lightwell_vulnerability_support_tickets
WHERE vulnerability_uuid = sqlc.arg(vulnerability_uuid)
    AND ticket_id <> ALL(sqlc.arg(ticket_ids)::text[]);

-- name: DeleteVulnerabilityCustomersNotIn :exec
DELETE FROM lightwell_vulnerability_customers
WHERE vulnerability_uuid = sqlc.arg(vulnerability_uuid)
    AND customer_id <> ALL(sqlc.arg(customer_ids)::text[]);
