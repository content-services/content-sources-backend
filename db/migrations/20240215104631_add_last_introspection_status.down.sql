BEGIN;

ALTER TABLE repositories
    RENAME COLUMN last_introspection_status to status;

COMMIT;
