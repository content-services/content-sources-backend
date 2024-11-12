BEGIN;

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS cancel_attempted BOOLEAN DEFAULT FALSE;

UPDATE tasks SET cancel_attempted = true WHERE status = 'canceled';

COMMIT;
