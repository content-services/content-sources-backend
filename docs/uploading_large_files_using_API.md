# Uploading large files using the Content Sources API

Use the [Red Hat Hybrid Cloud Console](https://console.redhat.com) Content Sources API to upload large RPM files in chunks and add them to an upload repository.

## References

- Content Sources OpenAPI spec: <https://console.redhat.com/api/content-sources/v1/openapi.json>
- Content Sources API in the Hybrid Cloud Console: <https://console.redhat.com/docs/api/content-sources>

## Prerequisites

- A Red Hat account with access to [Hybrid Cloud Console](https://console.redhat.com).
- A [service account](https://console.redhat.com/iam/service-accounts) in the same organization.

  A token only authenticates. The service account has no Content Sources privileges until you add it to a [User Access](https://console.redhat.com/iam/user-access/groups) group that includes the **Repositories administrator** role (or equivalent permissions). Service accounts are not in the Default access group.

  That role grants:

  - `content-sources:repositories:read` to list repositories and poll tasks
  - `content-sources:repositories:write` to create an upload repository
  - `content-sources:repositories:upload` to create uploads, send chunks, and add files to a repository

  Alternatively, combine **Repositories viewer**, a role that includes `content-sources:repositories:write`, and **Repositories uploader**.

- `curl`, `sha256sum` (Linux) or `shasum` (macOS), and `split`.

## Base URL and authentication

Set the API base URL and your service-account credentials. The major-version path `/api/content-sources/v1` also works.

```bash
export API="https://console.redhat.com/api/content-sources/v1.0"
export CLIENT_ID="<your-service-account-client-id>"
export CLIENT_SECRET="<your-service-account-client-secret>"
```

Request an access token (`client_credentials`):

```bash
export TOKEN=$(curl -sS --fail-with-body \
  -X POST "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=${CLIENT_ID}" \
  --data-urlencode "client_secret=${CLIENT_SECRET}" \
  --data-urlencode "scope=openid api.iam.service_accounts" \
  | jq -r .access_token)
```

Confirm the token is a JWT (it should start with `eyJ`, not `null`):

```bash
echo "$TOKEN" | head -c 20; echo
```

If this prints `null` or is empty, the token request failed. Print the SSO response without `jq` to see the error (wrong or unset `CLIENT_ID` / `CLIENT_SECRET` is the usual cause).

Check that the API is reachable. `/openapi.json` does not require a token:

```bash
curl -sS -o /dev/null -w "%{http_code}\n" "$API/openapi.json"
```

That should print `200`.

Confirm the service account has Content Sources permissions:

```bash
curl -sS -H "Authorization: Bearer $TOKEN" -H "Accept: application/json" \
  "https://console.redhat.com/api/rbac/v1/access/?application=content-sources"
```

You should see a list of permissions in the data section, including:

```json
[{"resourceDefinitions":[],"permission":"content-sources:repositories:write"},{"resourceDefinitions":[],"permission":"content-sources:repositories:read"},{"resourceDefinitions":[],"permission":"content-sources:repositories:upload"}]
```

An empty `"data": []` list means the token works, but the service account is not in a User Access group with those roles. Add any missing permissions under [User Access groups](https://console.redhat.com/iam/user-access/groups), then request a new token.

Then list repositories:

```bash
curl -sS -H "Authorization: Bearer $TOKEN" -H "Accept: application/json" \
  "$API/repositories/?limit=1"
```

A successful call returns a JSON collection (`data`, `meta`, `links`). Content Sources returns **401 Unauthorized** both for a missing token and for a valid token that lacks permission; use the RBAC `access` call above to tell those apart.

Tokens expire after about 15 minutes. Request a new token when a request returns `401`.

## Workflow

1. Prepare the RPM (checksum and chunks).
2. Create an **upload** repository, or reuse an existing one.
3. Create an upload session for the file.
4. Upload each chunk.
5. Add the completed upload to the repository.
6. Poll the Content Sources task until it finishes.

## 1. Prepare the file

Use any RPM you need to upload. This example use `example.rpm`.

Get the file size and SHA-256 checksum of the **entire** file. You send both when you create the upload, and you send the same checksum again when you add the upload to the repository.

Linux:

```bash
RPM=example.rpm
FILE_SIZE=$(stat -c%s "$RPM")
FILE_SHA256=$(sha256sum "$RPM" | awk '{print $1}')
echo "$FILE_SIZE $FILE_SHA256"
```

macOS:

```bash
RPM=example.rpm
FILE_SIZE=$(stat -f%z "$RPM")
FILE_SHA256=$(shasum -a 256 "$RPM" | awk '{print $1}')
echo "$FILE_SIZE $FILE_SHA256"
```

Split the file into chunks. This example use 6 MiB chunks (`6291456` bytes), which is the recommended size.

Set `CHUNK_DIR` to the directory that should hold the `chunk_*` files. The upload loop in step 4 must use the same directory. The default is the current working directory.

```bash
CHUNK_SIZE=6291456
CHUNK_DIR="${CHUNK_DIR:-.}"
mkdir -p "$CHUNK_DIR"
split -b "$CHUNK_SIZE" "$RPM" "$CHUNK_DIR/chunk_"
ls -l "$CHUNK_DIR"/chunk_*
```

`split` names chunks `chunk_aa`, `chunk_ab`, and so on. The last chunk can be smaller than `CHUNK_SIZE`.

## 2. Create an upload repository

Uploads can only be added to a repository with `"origin": "upload"` and `"snapshot": true`. You can also create this repository in the Hybrid Cloud Console UI (**Repositories** → create a repository of type **Upload**).

```bash
curl -s -X POST "$API/repositories/" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{
    "name": "my-upload-repo",
    "origin": "upload",
    "snapshot": true
  }'
```

Example response (fields omitted):

```json
{
  "uuid": "11111111-2222-3333-4444-555555555555",
  "name": "my-upload-repo",
  "origin": "upload",
  "snapshot": true,
  "last_snapshot_task_uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
}
```

Save the repository `uuid`. If the response includes `last_snapshot_task_uuid`, wait until that task completes before adding RPMs. Adding uploads while the repository is still being updated returns HTTP `409`.

```bash
REPO_UUID="<repository-uuid>"
TASK_UUID="<last_snapshot_task_uuid>"

curl -s -X GET "$API/tasks/$TASK_UUID" -H "Authorization: Bearer $TOKEN"
```

Wait until `"status"` is `"completed"`.

## 3. Create an upload

```bash
curl -s -X POST "$API/repositories/uploads/" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{
    \"size\": $FILE_SIZE,
    \"chunk_size\": $CHUNK_SIZE,
    \"sha256\": \"$FILE_SHA256\",
    \"resumable\": true
  }"
```

| Field | Required | Meaning |
| --- | --- | --- |
| `size` | yes | Total size of the RPM, in bytes |
| `chunk_size` | yes | Size of each chunk except possibly the last, in bytes |
| `sha256` | yes | SHA-256 of the **complete** RPM |
| `resumable` | no (send `true`) | Recommended default. If `true`, a later create request with the same `sha256`, `chunk_size`, and `size` reuses the existing upload and can list chunks already received. If omitted, the API treats it as `false` and starts a new upload. |

Example response:

```json
{
  "upload_uuid": "01906e9c-bd3b-74b2-a7b3-e122e9ca88d5",
  "created": "2024-07-01T14:04:44.220526Z",
  "last_updated": "2024-07-01T14:04:44.220541Z",
  "size": 62914560
}
```

Save `upload_uuid`. With `"resumable": true`, a retry after an interrupted upload can also include `completed_checksums` (SHA-256 values of chunks already stored) and, if the file was already fully stored, `artifact_href`.

## 4. Upload every chunk

You must upload **every** `chunk_*` file before `add_uploads`. Uploading only `chunk_aa` and then committing fails with `The sha256 checksum did not match`, because Pulp hashes the assembled file against `FILE_SHA256`.

Send each chunk with `POST`, `multipart/form-data`, and a `Content-Range` header:

- `file`: the chunk bytes
- `sha256`: SHA-256 of **this chunk**, not the whole file
- `Content-Range`: `bytes <start>-<end>/*`
  - `<start>` and `<end>` are inclusive byte offsets in the original file
  - `<end>` is one less than `start + chunk length`
  - use `*` for the total size (you already sent that when creating the upload)

Example for a file split into 6 MiB (`6291456` byte) chunks:

| Chunk | Size | Content-Range |
| --- | --- | --- |
| `chunk_aa` | 6291456 | `bytes 0-6291455/*` |
| `chunk_ab` | 6291456 | `bytes 6291456-12582911/*` |
| last | remainder | `bytes <start>-<FILE_SIZE-1>/*` |

Set `CHUNK_DIR` to the directory that contains the `chunk_*` files (the same directory used in step 1). The loop advances `offset` so each chunk is placed after the previous one. Do not reuse `START=0` for every chunk.

`FILE_SIZE` is derived from the rpm file in this script so it still works if you run the loop in a new terminal. Use the same rpm as in step 1 (the original RPM).

```bash
UPLOAD_UUID="<upload-uuid>"
RPM="${RPM:-example.rpm}"
FILE_SIZE=$(stat -c%s "$RPM")
CHUNK_DIR="${CHUNK_DIR:-.}"

offset=0
for chunk in "$CHUNK_DIR"/chunk_*; do
  chunk_size=$(stat -c%s "$chunk")
  end=$((offset + chunk_size - 1))
  chunk_sha256=$(sha256sum "$chunk" | awk '{print $1}')

  echo "Uploading $chunk as bytes ${offset}-${end}/*"
  curl -sS -X POST "$API/repositories/uploads/$UPLOAD_UUID/upload_chunk/" -H "Authorization: Bearer $TOKEN" -H "Content-Range: bytes ${offset}-${end}/*" --form "file=@${chunk}" --form "sha256=${chunk_sha256}"
  echo

  offset=$((offset + chunk_size))
done

if [ "$offset" -ne "$FILE_SIZE" ]; then
  echo "error: uploaded $offset bytes, expected $FILE_SIZE" >&2
  exit 1
fi
```

On macOS, replace `stat -c%s` with `stat -f%z` and `sha256sum` with `shasum -a 256`.

Example chunk response:

```json
{
  "upload_uuid": "01906e9c-bd3b-74b2-a7b3-e122e9ca88d5",
  "created": "2024-07-01T14:44:58.435098Z",
  "last_updated": "2024-07-01T14:44:58.435113Z",
  "size": 62914560
}
```

## 5. Add the upload to the repository

After every chunk is uploaded, add the upload UUID and the **whole-file** SHA-256 to the repository. This step commits the file, turns it into an RPM in the repository, and starts a snapshot.

```bash
curl -s -X POST "$API/repositories/$REPO_UUID/add_uploads/" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{
    \"uploads\": [
      {
        \"uuid\": \"$UPLOAD_UUID\",
        \"sha256\": \"$FILE_SHA256\"
      }
    ]
  }"
```

You can include several uploads in one request. Use `uuid` (the upload ID). Do not send Pulp `href` values; those are only used by internal services.

Example response:

```json
{
  "uuid": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff",
  "status": "running",
  "created_at": "2024-07-01T14:09:40.609565Z",
  "ended_at": "",
  "error": "",
  "org_id": "12345678",
  "type": "add-uploads-repository",
  "object_type": "repository",
  "object_name": "my-upload-repo",
  "object_uuid": "11111111-2222-3333-4444-555555555555"
}
```

A `409` response means the repository is already being updated. Wait for the in-progress task, then retry.

If the add-uploads task fails with `The sha256 checksum did not match`, the assembled bytes were not the original RPM. Usual causes:

- not every `chunk_*` file was uploaded
- every chunk was sent with `Content-Range: bytes 0-…` instead of advancing the offset
- `sha256` on `add_uploads` was a chunk checksum instead of `FILE_SHA256`

Create a **new** upload (the failed commit removes the previous one), upload every chunk with the loop above, then call `add_uploads` again.

## 6. Check task status

```bash
ADD_TASK_UUID="<task-uuid-from-add_uploads>"

curl -s -X GET "$API/tasks/$ADD_TASK_UUID" -H "Authorization: Bearer $TOKEN"
```

`status` is one of `pending`, `running`, `completed`, `failed`, or `canceled`. When the task completes, the RPM is available in the repository.

List RPMs in the repository:

```bash
curl -s -X GET "$API/repositories/$REPO_UUID/rpms" -H "Authorization: Bearer $TOKEN"
```

## Notes

- When creating an upload (step 3) and adding it with `add_uploads`, `sha256` is the checksum of the **complete** RPM. Chunk `sha256` is the checksum of **that chunk**.
- `Content-Range` end offsets are inclusive. For a file of size `N`, the last byte index is `N-1`.
- Use the same `CHUNK_DIR` when you split the file (step 1) and when you upload the chunks (step 4).
- Call `add_uploads` only after the loop has uploaded a total of `FILE_SIZE` bytes.
- These are the Content Sources paths used:

  | Step | Method and path |
  | --- | --- |
  | Create repository | `POST /api/content-sources/v1.0/repositories/` |
  | Create upload | `POST /api/content-sources/v1.0/repositories/uploads/` |
  | Upload chunk | `POST /api/content-sources/v1.0/repositories/uploads/{upload_uuid}/upload_chunk/` |
  | Add to repository | `POST /api/content-sources/v1.0/repositories/{uuid}/add_uploads/` |
  | Get task | `GET /api/content-sources/v1.0/tasks/{uuid}` |

