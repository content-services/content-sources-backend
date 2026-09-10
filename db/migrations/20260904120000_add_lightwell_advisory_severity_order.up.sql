BEGIN;

ALTER TABLE lightwell_advisories
    ADD COLUMN IF NOT EXISTS severity_score REAL NOT NULL DEFAULT 0;

UPDATE lightwell_advisories
SET severity_score = CASE
    WHEN severity ~ '^[0-9]+\.?[0-9]*$' THEN severity::real
    ELSE 0
END;

CREATE INDEX IF NOT EXISTS idx_lightwell_advisories_severity_score
    ON lightwell_advisories (severity_score);

CREATE INDEX IF NOT EXISTS idx_lightwell_advisories_package_name
    ON lightwell_advisories (package_name);

COMMIT;
