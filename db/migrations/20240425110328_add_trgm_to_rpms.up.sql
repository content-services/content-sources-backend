BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS rpms_trgm_gin ON rpms USING gin (name gin_trgm_ops);

COMMIT;
