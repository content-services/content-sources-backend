BEGIN;

ALTER TABLE coverage_report_packages
    DROP COLUMN IF EXISTS cve_critical,
    DROP COLUMN IF EXISTS cve_high,
    DROP COLUMN IF EXISTS cve_medium,
    DROP COLUMN IF EXISTS cve_low,
    DROP COLUMN IF EXISTS cve_range_low,
    DROP COLUMN IF EXISTS cve_range_high;

ALTER TABLE coverage_reports
    DROP COLUMN IF EXISTS cve_critical,
    DROP COLUMN IF EXISTS cve_high,
    DROP COLUMN IF EXISTS cve_medium,
    DROP COLUMN IF EXISTS cve_low;

COMMIT;
