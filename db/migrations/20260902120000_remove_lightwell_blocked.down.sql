BEGIN;

CREATE OR REPLACE FUNCTION lightwell_filtered_vulnerabilities(
    p_customer_id TEXT,
    p_severities TEXT[],
    p_stages TEXT[],
    p_complexities TEXT[],
    p_ltwlsupt_ticket_ids TEXT[],
    p_flags TEXT[],
    p_search TEXT
) RETURNS SETOF lightwell_vulnerabilities
LANGUAGE sql
STABLE
AS $$
SELECT v.*
FROM lightwell_vulnerabilities v
INNER JOIN lightwell_vulnerability_customers vc ON vc.vulnerability_uuid = v.uuid
WHERE vc.customer_id = p_customer_id
    AND (
        p_severities IS NULL
        OR cardinality(p_severities) = 0
        OR v.severity = ANY (p_severities)
    )
    AND (
        p_stages IS NULL
        OR cardinality(p_stages) = 0
        OR v.stage = ANY (p_stages)
    )
    AND (
        p_complexities IS NULL
        OR cardinality(p_complexities) = 0
        OR v.complexity = ANY (p_complexities)
    )
    AND (
        p_ltwlsupt_ticket_ids IS NULL
        OR cardinality(p_ltwlsupt_ticket_ids) = 0
        OR EXISTS (
            SELECT 1
            FROM lightwell_vulnerability_support_tickets t
            WHERE t.vulnerability_uuid = v.uuid
                AND t.customer_id = p_customer_id
                AND t.ticket_id = ANY (p_ltwlsupt_ticket_ids)
        )
    )
    AND (
        p_flags IS NULL
        OR cardinality(p_flags) = 0
        OR (
            ('embargo' = ANY (p_flags) AND v.embargo = true)
            OR ('duplicate' = ANY (p_flags) AND v.duplicate = true)
            OR (
                'blocked' = ANY (p_flags)
                AND v.stage <> 'Lightwell Network'
                AND (CURRENT_DATE - v.submitted_date) > 30
            )
        )
    )
    AND (
        p_search IS NULL
        OR v.vulnerability_id ILIKE '%' || p_search || '%'
        OR v.component_name ILIKE '%' || p_search || '%'
        OR v.title ILIKE '%' || p_search || '%'
    )
$$;

CREATE INDEX idx_lv_blocked ON lightwell_vulnerabilities (submitted_date, stage);

COMMIT;
