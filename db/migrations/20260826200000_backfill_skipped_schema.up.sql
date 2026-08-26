BEGIN;

-- Backfill: coverage_reports and related tables (from 20260814162834)
-- These migrations were added after the DB had already advanced past their
-- version numbers, so golang-migrate skipped them.

CREATE TABLE IF NOT EXISTS coverage_reports (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    org_id VARCHAR(255) NOT NULL,
    account_id VARCHAR(255),
    status VARCHAR(255) NOT NULL,
    input_format VARCHAR(255),
    total INTEGER,
    exact_matches INTEGER,
    partial_matches INTEGER,
    unmatched INTEGER,
    ecosystem_coverage_summary JSONB,
    catalog_snapshot_at TIMESTAMP WITH TIME ZONE,
    analysis_task_error TEXT,
    analysis_task_uuid UUID,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_coverage_reports_org_id ON coverage_reports (org_id);

CREATE TABLE IF NOT EXISTS coverage_report_packages (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    coverage_report_uuid UUID NOT NULL,
    ecosystem VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    namespace VARCHAR(255),
    match_status VARCHAR(255) NOT NULL
);

ALTER TABLE ONLY coverage_report_packages
    DROP CONSTRAINT IF EXISTS fk_coverage_report_packages_coverage_report,
    ADD CONSTRAINT fk_coverage_report_packages_coverage_report
        FOREIGN KEY (coverage_report_uuid) REFERENCES coverage_reports(uuid) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_coverage_report_packages_report_uuid
    ON coverage_report_packages (coverage_report_uuid);

CREATE TABLE IF NOT EXISTS coverage_demand_signals (
    uuid UUID UNIQUE NOT NULL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    ecosystem VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    namespace VARCHAR(255),
    match_status VARCHAR(255) NOT NULL,
    source VARCHAR(255) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_coverage_demand_signals_ecosystem_namespace_name_version
    ON coverage_demand_signals (ecosystem, namespace, name, version);

-- Backfill: duplicate_of column on lightwell_vulnerabilities (from 20260818120000)

ALTER TABLE lightwell_vulnerabilities ADD COLUMN IF NOT EXISTS duplicate_of TEXT;

COMMIT;
