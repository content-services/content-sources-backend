BEGIN;

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

CREATE INDEX idx_lvc_customer_id ON lightwell_vulnerability_customers (customer_id);

CREATE INDEX idx_lv_severity ON lightwell_vulnerabilities (severity);
CREATE INDEX idx_lv_stage ON lightwell_vulnerabilities (stage);
CREATE INDEX idx_lv_complexity ON lightwell_vulnerabilities (complexity);
CREATE INDEX idx_lv_ltwwlsupt_ticket_id ON lightwell_vulnerabilities (ltwwlsupt_ticket_id);

CREATE INDEX idx_lv_embargo_true ON lightwell_vulnerabilities (uuid) WHERE embargo = true;
CREATE INDEX idx_lv_duplicate_true ON lightwell_vulnerabilities (uuid) WHERE duplicate = true;

CREATE INDEX idx_lv_blocked ON lightwell_vulnerabilities (submitted_date, stage);

CREATE INDEX idx_lv_vulnerability_id_trgm ON lightwell_vulnerabilities USING gin (vulnerability_id gin_trgm_ops);
CREATE INDEX idx_lv_component_name_trgm ON lightwell_vulnerabilities USING gin (component_name gin_trgm_ops);
CREATE INDEX idx_lv_title_trgm ON lightwell_vulnerabilities USING gin (title gin_trgm_ops);

COMMIT;
