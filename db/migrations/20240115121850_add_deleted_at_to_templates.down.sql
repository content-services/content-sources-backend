BEGIN;
ALTER TABLE templates DROP COLUMN deleted_at;
COMMIT;
