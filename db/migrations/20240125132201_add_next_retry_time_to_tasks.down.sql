BEGIN;

ALTER TABLE tasks DROP COLUMN next_retry_time;

COMMIT;
