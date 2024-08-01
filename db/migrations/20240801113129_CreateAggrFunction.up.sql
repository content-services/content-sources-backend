BEGIN;

DROP AGGREGATE IF EXISTS array_concat_agg(anycompatiblearray);
CREATE AGGREGATE array_concat_agg(anycompatiblearray) (
				SFUNC = array_cat,
				STYPE = anycompatiblearray
			);

COMMIT;
