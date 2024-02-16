BEGIN;

DROP INDEX name_org_id_not_deleted_unique;
ALTER TABLE templates
    ADD CONSTRAINT name_org_id_unique UNIQUE (name, org_id);

COMMIT;
