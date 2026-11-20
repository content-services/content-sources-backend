BEGIN;

ALTER TABLE coverage_report_packages
    DROP COLUMN IF EXISTS lightwell_versions,
    DROP COLUMN IF EXISTS nearest_higher_version,
    DROP COLUMN IF EXISTS match_segment;

COMMIT;
