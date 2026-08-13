BEGIN;

ALTER TABLE lightwell_access_tokens
  ADD COLUMN IF NOT EXISTS access_level VARCHAR(32) NOT NULL DEFAULT 'validated';

ALTER TABLE lightwell_access_tokens
  DROP CONSTRAINT IF EXISTS lightwell_access_tokens_access_level_check;

ALTER TABLE lightwell_access_tokens
  ADD CONSTRAINT lightwell_access_tokens_access_level_check
  CHECK (access_level IN ('validated', 'remediated'));

COMMIT;
