BEGIN;

ALTER TABLE templates
    DROP CONSTRAINT name_org_id_unique;

CREATE UNIQUE INDEX IF NOT EXISTS name_org_id_not_deleted_unique
    ON templates(name, org_id)
    WHERE deleted_at IS NULL;

COMMIT;
