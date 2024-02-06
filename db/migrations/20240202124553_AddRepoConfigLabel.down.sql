BEGIN;
ALTER TABLE repository_configurations DROP COLUMN label;
COMMIT;
