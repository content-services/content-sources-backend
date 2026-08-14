-- sqlc schema snapshot: current lightwell vulnerabilities tables (see db/migrations)

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
