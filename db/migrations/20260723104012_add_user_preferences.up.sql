BEGIN;

CREATE TABLE IF NOT EXISTS user_preferences (
  uuid UUID UNIQUE NOT NULL PRIMARY KEY,
  org_id VARCHAR(255) NOT NULL,
  user_id VARCHAR(255) NOT NULL,
  label VARCHAR(255) NOT NULL,
  value TEXT NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  UNIQUE (org_id, user_id, label)
);

CREATE INDEX IF NOT EXISTS user_preferences_org_user_idx ON user_preferences (org_id, user_id);

COMMIT;
