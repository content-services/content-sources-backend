-- name: RecordBeaconSync :exec
INSERT INTO lightwell_beacon_sync (id, last_processed_at)
VALUES (true, sqlc.arg(last_processed_at))
ON CONFLICT (id) DO UPDATE SET
    last_processed_at = EXCLUDED.last_processed_at;
