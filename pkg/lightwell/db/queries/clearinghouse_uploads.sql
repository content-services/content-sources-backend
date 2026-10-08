-- name: CreateClearinghouseUpload :one
INSERT INTO clearinghouse_uploads (
    uuid,
    org_id,
    account_id,
    vulnerability_count,
    status,
    error_message
) VALUES (
    sqlc.arg(uuid),
    sqlc.arg(org_id),
    sqlc.narg(account_id),
    sqlc.narg(vulnerability_count),
    sqlc.arg(status),
    sqlc.narg(error_message)
)
RETURNING *;

-- name: GetClearinghouseUpload :one
SELECT *
FROM clearinghouse_uploads
WHERE uuid = sqlc.arg(uuid);

-- name: CreateClearinghouseUploadFile :one
INSERT INTO clearinghouse_upload_files (
    uuid,
    clearinghouse_upload_uuid,
    filename,
    filepath
) VALUES (
    sqlc.arg(uuid),
    sqlc.arg(clearinghouse_upload_uuid),
    sqlc.arg(filename),
    sqlc.arg(filepath)
)
RETURNING *;

-- name: ListClearinghouseUploadFiles :many
SELECT *
FROM clearinghouse_upload_files
WHERE clearinghouse_upload_uuid = sqlc.arg(clearinghouse_upload_uuid)
ORDER BY filename;
