-- sqlc schema snapshot: current lightwell vulnerabilities tables (see db/migrations)
-- NOTE: This file must be kept in sync with the actual migrations.
-- sqlc uses this for code generation; it is not executed directly.

CREATE TABLE repository_configurations (
    uuid UUID PRIMARY KEY,
    feature_name VARCHAR(255) DEFAULT NULL
);

CREATE TABLE lightwell_advisories (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    repo_name TEXT NOT NULL,
    advisory_id TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT '',
    severity_score REAL NOT NULL DEFAULT 0,
    details TEXT NOT NULL DEFAULT '',
    reference_urls TEXT[],
    package_name TEXT NOT NULL DEFAULT '',
    fixed_version TEXT NOT NULL DEFAULT '',
    fixed_versions TEXT[] NOT NULL DEFAULT '{}',
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    checksum TEXT NOT NULL,
    published TIMESTAMPTZ NULL,
    modified TIMESTAMPTZ NULL,
    aliases TEXT[] NOT NULL DEFAULT '{}',
    schema_version TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    package_version TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_advisories_repo_config_advisory
    ON lightwell_advisories (repository_configuration_uuid, advisory_id, package_name);
CREATE INDEX idx_lightwell_advisories_severity_score
    ON lightwell_advisories (severity_score);
CREATE INDEX idx_lightwell_advisories_package_name
    ON lightwell_advisories (package_name);
CREATE INDEX idx_lightwell_advisories_advisory_id_trgm
    ON lightwell_advisories USING gin (advisory_id gin_trgm_ops);

CREATE TABLE lightwell_advisory_releases (
    advisory_uuid UUID NOT NULL REFERENCES lightwell_advisories(uuid) ON DELETE CASCADE,
    release_version TEXT NOT NULL,
    rhlw_baseline INT NOT NULL DEFAULT 0,
    rhlw_novel INT NOT NULL DEFAULT 0,
    rhlw_hotfix INT NOT NULL DEFAULT 0,
    PRIMARY KEY (advisory_uuid, release_version)
);
CREATE INDEX idx_lightwell_advisory_releases_rank
    ON lightwell_advisory_releases (rhlw_baseline DESC, rhlw_novel DESC, rhlw_hotfix DESC);

CREATE TABLE lightwell_vulnerabilities (
    uuid UUID PRIMARY KEY,
    vulnerability_key TEXT NOT NULL UNIQUE,
    vulnerability_id TEXT NOT NULL,
    purl TEXT,
    component_name TEXT NOT NULL,
    component_version TEXT NOT NULL,
    published_versions TEXT[] NOT NULL DEFAULT '{}',
    title TEXT,
    cwe TEXT,
    description TEXT,
    severity TEXT NOT NULL,
    cvss DOUBLE PRECISION,
    cvss_vector TEXT,
    exploit_tested BOOLEAN NOT NULL DEFAULT false,
    reproducer_included BOOLEAN NOT NULL DEFAULT false,
    customer_priority TEXT,
    stage TEXT NOT NULL,
    language TEXT,
    complexity TEXT NOT NULL,
    submitted_date DATE NOT NULL,
    last_updated TIMESTAMPTZ NOT NULL,
    embargo BOOLEAN NOT NULL DEFAULT false,
    duplicate BOOLEAN NOT NULL DEFAULT false,
    duplicate_of TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE lightwell_vulnerability_customers (
    customer_id TEXT NOT NULL,
    vulnerability_uuid UUID NOT NULL REFERENCES lightwell_vulnerabilities (uuid) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, vulnerability_uuid)
);

CREATE TABLE lightwell_vulnerability_support_tickets (
    vulnerability_uuid UUID NOT NULL,
    customer_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (vulnerability_uuid, ticket_id),
    FOREIGN KEY (customer_id, vulnerability_uuid)
        REFERENCES lightwell_vulnerability_customers (customer_id, vulnerability_uuid)
        ON DELETE CASCADE
);

CREATE FUNCTION lightwell_filtered_vulnerabilities(
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
        )
    )
    AND (
        p_search IS NULL
        OR v.vulnerability_id ILIKE '%' || p_search || '%'
        OR v.component_name ILIKE '%' || p_search || '%'
        OR v.title ILIKE '%' || p_search || '%'
    )
$$;
