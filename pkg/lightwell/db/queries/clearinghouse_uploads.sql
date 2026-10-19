-- name: CreateClearinghouseUpload :one
INSERT INTO clearinghouse_uploads (
    uuid,
    org_id,
    username,
    filename,
    vulnerability_count,
    status,
    error_message
) VALUES (
    sqlc.arg(uuid),
    sqlc.arg(org_id),
    sqlc.arg(username),
    sqlc.arg(filename),
    sqlc.narg(vulnerability_count),
    sqlc.arg(status),
    sqlc.narg(error_message)
)
RETURNING *;

-- name: GetClearinghouseUpload :one
SELECT *
FROM clearinghouse_uploads
WHERE uuid = sqlc.arg(uuid);

-- name: GetClearinghouseUploadForUpdate :one
SELECT *
FROM clearinghouse_uploads
WHERE uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: CreateClearinghouseUploadFile :one
INSERT INTO clearinghouse_upload_files (
    uuid,
    clearinghouse_upload_uuid,
    filename,
    filepath,
    username
) VALUES (
    sqlc.arg(uuid),
    sqlc.arg(clearinghouse_upload_uuid),
    sqlc.arg(filename),
    sqlc.arg(filepath),
    sqlc.arg(username)
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

-- name: DeleteClearinghouseUpload :exec
DELETE FROM clearinghouse_uploads
WHERE uuid = sqlc.arg(uuid);

-- name: DeleteClearinghouseUploadFile :exec
DELETE FROM clearinghouse_upload_files
WHERE uuid = sqlc.arg(uuid);

-- name: CountClearinghouseUploads :one
SELECT COUNT(*)::bigint
FROM clearinghouse_uploads
WHERE sqlc.narg(org_id)::text IS NULL OR org_id = sqlc.narg(org_id)::text;

-- name: SubmitClearinghouseUpload :one
UPDATE clearinghouse_uploads
SET status = 'submitted',
    submitted_at = NOW(),
    submitted_by = sqlc.arg(submitted_by)
WHERE uuid = sqlc.arg(uuid)
    AND status = 'uploading'
RETURNING *;

-- name: ProcessClearinghouseUpload :one
UPDATE clearinghouse_uploads
SET status = 'processed',
    processed_at = NOW(),
    processed_by = sqlc.arg(processed_by)
WHERE uuid = sqlc.arg(uuid)
    AND status = 'submitted'
RETURNING *;
