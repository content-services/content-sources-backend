BEGIN;

DO $$

BEGIN

IF EXISTS (
    SELECT * 
    FROM information_schema.columns 
    WHERE table_name='repositories' AND column_name='status'
) THEN
    ALTER TABLE repositories RENAME COLUMN status to last_introspection_status;
END IF;

END $$;

COMMIT;