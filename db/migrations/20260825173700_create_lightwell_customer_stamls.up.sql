BEGIN;

CREATE TABLE lightwell_customer_stamls (
    customer_id TEXT NOT NULL,
    staml TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, staml)
);

CREATE INDEX idx_lcs_staml ON lightwell_customer_stamls (staml);

COMMIT;
