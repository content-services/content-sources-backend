BEGIN;

CREATE TABLE IF NOT EXISTS lightwell_access_tokens (
  uuid UUID UNIQUE NOT NULL PRIMARY KEY,
  org_id VARCHAR(255) NOT NULL,
  user_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  token_prefix VARCHAR(32) NOT NULL,
  token_hash VARCHAR(128) NOT NULL UNIQUE,
  expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
  revoked_at TIMESTAMP WITH TIME ZONE,
  last_used_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS lightwell_access_tokens_org_idx ON lightwell_access_tokens (org_id);
CREATE INDEX IF NOT EXISTS lightwell_access_tokens_org_user_idx ON lightwell_access_tokens (org_id, user_id);

COMMIT;
