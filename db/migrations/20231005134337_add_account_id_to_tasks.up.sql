BEGIN;

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS account_id varchar;

CREATE INDEX IF NOT EXISTS tasks_account_id
    ON tasks(account_id);

COMMIT;
