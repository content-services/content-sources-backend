BEGIN;

ALTER TABLE repositories ADD COLUMN IF NOT EXISTS origin varchar default 'external' not null;
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS content_type varchar default 'rpm' not null;

COMMIT;
