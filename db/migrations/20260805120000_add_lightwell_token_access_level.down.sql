BEGIN;

ALTER TABLE lightwell_access_tokens
  DROP CONSTRAINT IF EXISTS lightwell_access_tokens_access_level_check;

ALTER TABLE lightwell_access_tokens
  DROP COLUMN IF EXISTS access_level;

COMMIT;
