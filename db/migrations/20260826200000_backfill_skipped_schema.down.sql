BEGIN;

ALTER TABLE lightwell_vulnerabilities DROP COLUMN IF EXISTS duplicate_of;

DROP TABLE IF EXISTS coverage_demand_signals;
DROP TABLE IF EXISTS coverage_report_packages;
DROP TABLE IF EXISTS coverage_reports;

COMMIT;
