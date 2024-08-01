BEGIN;
DROP AGGREGATE IF EXISTS array_concat_agg(anycompatiblearray);
COMMIT;
