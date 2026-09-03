BEGIN;

ALTER TABLE lightwell_advisories
    ADD COLUMN IF NOT EXISTS severity_order SMALLINT NOT NULL DEFAULT 0;

UPDATE lightwell_advisories SET severity_order = CASE
    WHEN severity = 'critical' THEN 4
    WHEN severity = 'important' THEN 3
    WHEN severity = 'moderate' THEN 2
    WHEN severity = 'low' THEN 1
    ELSE 0
END;

CREATE INDEX IF NOT EXISTS idx_lightwell_advisories_severity_order
    ON lightwell_advisories (severity_order);

CREATE INDEX IF NOT EXISTS idx_lightwell_advisories_package_name
    ON lightwell_advisories (package_name);

COMMIT;
