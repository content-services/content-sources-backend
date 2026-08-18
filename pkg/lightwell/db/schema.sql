-- sqlc schema snapshot: lightwell tables (see db/migrations)
-- NOTE: This file must be kept in sync with the actual migrations.
-- sqlc uses this for code generation; it is not executed directly.

CREATE TABLE repository_configurations (
    uuid UUID PRIMARY KEY
);

CREATE TABLE lightwell_advisories (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    repo_name VARCHAR(255) NOT NULL,
    advisory_id VARCHAR(255) NOT NULL,
    severity VARCHAR(255) NOT NULL DEFAULT '',
    severity_order SMALLINT NOT NULL DEFAULT 0,
    details TEXT NOT NULL DEFAULT '',
    reference_urls TEXT[],
    package_name VARCHAR(255) NOT NULL DEFAULT '',
    fixed_version VARCHAR(255) NOT NULL DEFAULT '',
    fixed_versions TEXT[] NOT NULL DEFAULT '{}',
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    checksum VARCHAR(255) NOT NULL
);

CREATE UNIQUE INDEX idx_lightwell_advisories_repo_config_advisory
    ON lightwell_advisories (repository_configuration_uuid, advisory_id, package_name);
CREATE INDEX idx_lightwell_advisories_severity_order
    ON lightwell_advisories (severity_order);
CREATE INDEX idx_lightwell_advisories_package_name
    ON lightwell_advisories (package_name);

CREATE TABLE lightwell_vulnerabilities (
    uuid UUID PRIMARY KEY,
    vulnerability_id TEXT NOT NULL UNIQUE,
    purl TEXT,
    component_name TEXT NOT NULL,
    component_version TEXT NOT NULL,
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
    ltwwlsupt_ticket_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE lightwell_vulnerability_customers (
    customer_id TEXT NOT NULL,
    vulnerability_uuid UUID NOT NULL REFERENCES lightwell_vulnerabilities (uuid) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, vulnerability_uuid)
);
