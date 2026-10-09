ALTER TABLE coverage_reports ADD COLUMN IF NOT EXISTS skipped_entries INTEGER CHECK (skipped_entries >= 0);
