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

-- name: ListClearinghouseUploadFilesByUploads :many
SELECT *
FROM clearinghouse_upload_files
WHERE clearinghouse_upload_uuid = ANY(sqlc.arg(upload_uuids)::uuid[])
ORDER BY clearinghouse_upload_uuid, filename;

-- name: GetClearinghouseUploadFileByName :one
SELECT *
FROM clearinghouse_upload_files
WHERE clearinghouse_upload_uuid = sqlc.arg(clearinghouse_upload_uuid)
    AND filename = sqlc.arg(filename);

-- name: ListClearinghouseUploads :many
SELECT *
FROM clearinghouse_uploads
WHERE sqlc.narg(org_id)::text IS NULL OR org_id = sqlc.narg(org_id)::text
ORDER BY created_at DESC, uuid DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountClearinghouseUploads :one
SELECT COUNT(*)::bigint
FROM clearinghouse_uploads
WHERE sqlc.narg(org_id)::text IS NULL OR org_id = sqlc.narg(org_id)::text;
