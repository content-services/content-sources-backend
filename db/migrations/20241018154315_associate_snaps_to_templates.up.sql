BEGIN;

ALTER TABLE templates_repository_configurations ADD COLUMN IF NOT EXISTS snapshot_uuid UUID;

-- migrate snapshots for use_latest templates
UPDATE templates_repository_configurations trc
SET snapshot_uuid = (
    SELECT rc.last_snapshot_uuid
    FROM repository_configurations rc
             JOIN templates t ON t.uuid = trc.template_uuid
    WHERE rc.uuid = trc.repository_configuration_uuid
      AND t.use_latest = true
      AND rc.last_snapshot_uuid IS NOT NULL
)
WHERE trc.template_uuid IN (
    SELECT t.uuid
    FROM templates t
    WHERE t.use_latest = true
);

-- migrate snapshots for templates with a snapshot date
UPDATE templates_repository_configurations trc
SET snapshot_uuid = (
    SELECT closest_snapshots.uuid
    FROM (
             (SELECT s.uuid, s.created_at
              FROM snapshots s
                       JOIN templates t ON t.uuid = trc.template_uuid
              WHERE s.repository_configuration_uuid = trc.repository_configuration_uuid
                AND t.use_latest = false
                AND s.created_at <= t.date
              ORDER BY s.created_at DESC
                  LIMIT 1)

             UNION

             (SELECT s.uuid, s.created_at
              FROM snapshots s
                       JOIN templates t ON t.uuid = trc.template_uuid
              WHERE s.repository_configuration_uuid = trc.repository_configuration_uuid
                AND t.use_latest = false
                AND s.created_at > t.date
              ORDER BY s.created_at ASC
                  LIMIT 1)
         ) AS closest_snapshots
    ORDER BY closest_snapshots.created_at ASC
    LIMIT 1
)
WHERE trc.template_uuid IN (
    SELECT t.uuid
    FROM templates t
    WHERE t.use_latest = false
    );

DELETE FROM templates_repository_configurations WHERE snapshot_uuid IS NULL;

ALTER TABLE templates_repository_configurations ALTER COLUMN snapshot_uuid SET NOT NULL;

ALTER TABLE templates_repository_configurations
DROP CONSTRAINT IF EXISTS fk_templates_repository_configurations_snapshots,
ADD CONSTRAINT fk_templates_repository_configurations_snapshots
FOREIGN KEY (snapshot_uuid) REFERENCES snapshots(uuid)
ON DELETE RESTRICT;

COMMIT;