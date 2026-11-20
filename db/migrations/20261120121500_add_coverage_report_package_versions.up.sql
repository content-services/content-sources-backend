BEGIN;

ALTER TABLE coverage_report_packages
    ADD COLUMN IF NOT EXISTS lightwell_versions TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS nearest_higher_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS match_segment TEXT NOT NULL DEFAULT 'none';

COMMIT;
