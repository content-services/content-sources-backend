BEGIN;

CREATE TABLE lightwell_beacon_sync (
    id BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    last_processed_at TIMESTAMPTZ NOT NULL
);

COMMIT;
