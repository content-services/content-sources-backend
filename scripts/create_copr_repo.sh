#!/usr/bin/env bash
#
# Sets up Pulp the way Copr does for a project chroot, so the Copr build end
# messages in test/kafka/copr can be tested end to end (consumer -> task -> snapshot).
#
# In the Copr domain (clients.pulp.copr_domain, "cs-partner" by default) it creates
# one RPM repository for the "@copr/example" project's rhel-9-x86_64 chroot, syncs it
# twice to simulate two builds (each build adds a repository version), publishes
# both versions, and serves the latest one through a distribution, as Copr does.
#
# Then it writes the real repository and publication hrefs, and the current time as the
# CloudEvent time (so the event delay metric is recorded), into the payloads:
#   build-end.json              -> build 1001, publication of version 1
#   build-end-second-build.json -> build 1006, publication of version 2 (same repository)
#
# Idempotent: the domain, remotes, repository, publications, and distribution are
# reused when they already exist, so re-running gives the same hrefs.
#
# Intended for local development against the Pulp instance started via docker-compose.
#
# Usage:
#   ./scripts/create_copr_repo.sh [--payload-dir DIR]
#
# Auth:
#   Basic auth (default): set PULP_USER and PULP_PASS
#   Cert auth: set PULP_CLIENT_CERT and PULP_CLIENT_KEY (and optionally PULP_CA_CERT)
#              Leave PULP_USER unset or empty to use cert auth.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration (override with environment variables)
# ---------------------------------------------------------------------------

PULP_URL="${PULP_URL:-https://localhost:8443}"
PULP_USER="${PULP_USER:-}"
PULP_PASS="${PULP_PASS:-}"
PULP_CLIENT_CERT="${PULP_CLIENT_CERT:-}"
PULP_CLIENT_KEY="${PULP_CLIENT_KEY:-}"
PULP_CA_CERT="${PULP_CA_CERT:-}"
DOMAIN="${DOMAIN:-cs-partner}"
# Content of the first and second build; the second is a superset of the first
BUILD_1_REMOTE_URL="${BUILD_1_REMOTE_URL:-https://content-services.github.io/fixtures/yum/comps-modules/v1/}"
BUILD_2_REMOTE_URL="${BUILD_2_REMOTE_URL:-https://content-services.github.io/fixtures/yum/comps-modules/v2/}"
API_ROOT="/api/pulp"
REPO_DIR="$(cd "$(dirname "$0")/.."; pwd)"
PAYLOAD_DIR="${REPO_DIR}/test/kafka/copr"

# Must match the owner, copr, and chroot in the payloads
NAME="copr-example-rhel-9-x86_64"
BASE_PATH="copr/example/rhel-9-x86_64"

# If no user is set, default to basic auth with admin/password for backwards compat
if [[ -z "$PULP_USER" && -z "$PULP_CLIENT_CERT" ]]; then
  PULP_USER="admin"
  PULP_PASS="password"
fi

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --payload-dir) PAYLOAD_DIR="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Joins PULP_URL and a path without producing a double slash.
pulp_url() {
  local path="$1"
  printf '%s/%s\n' "${PULP_URL%/}" "${path#/}"
}

# Calls the Pulp REST API and returns the JSON response body.
# Uses cert auth when PULP_CLIENT_CERT is set, otherwise basic auth.
#   pulp_api <METHOD> <PATH> [BODY]
pulp_api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"

  local -a args
  args=(-s -k -X "${method}" -H "Content-Type: application/json")
  if [[ -n "$PULP_CLIENT_CERT" ]]; then
    args+=(--cert "${PULP_CLIENT_CERT}")
    if [[ -n "$PULP_CLIENT_KEY" ]]; then
      args+=(--key "${PULP_CLIENT_KEY}")
    fi
    if [[ -n "$PULP_CA_CERT" ]]; then
      args+=(--cacert "${PULP_CA_CERT}")
    fi
  elif [[ -n "$PULP_USER" ]]; then
    args+=(-u "${PULP_USER}:${PULP_PASS}")
  fi

  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi

  curl "${args[@]}" "$(pulp_url "$path")"
}

# Calls pulp_api to POST (or another method) a resource, extracts a field from the
# response, and exits with a clear error if the field is missing.
#   pulp_create <LABEL> <PATH> <BODY> <JQ_FIELD> [METHOD]
#   Sets RESULT to the extracted value.
pulp_create() {
  local label="$1"
  local path="$2"
  local body="$3"
  local field="$4"
  local method="${5:-POST}"

  local response
  response=$(pulp_api "$method" "$path" "$body")

  RESULT=$(echo "$response" | jq -r "$field")

  if [[ "$RESULT" == "null" || -z "$RESULT" ]]; then
    echo "ERROR: Failed to create ${label}:" >&2
    echo "$response" | jq . >&2
    exit 1
  fi
}

# Polls a Pulp task until it reaches a terminal state. Sets RESULT to the first
# created resource (empty if the task created none).
#   wait_for_task <TASK_HREF>
wait_for_task() {
  local task_href="$1"
  local max_attempts=240
  local state=""
  local i

  for ((i = 1; i <= max_attempts; i++)); do
    local task_response
    task_response=$(pulp_api GET "$task_href")
    state=$(echo "$task_response" | jq -r '.state')

    case "$state" in
      completed)
        printf "\n"
        RESULT=$(echo "$task_response" | jq -r '.created_resources[0] // empty')
        return 0
        ;;
      failed|canceled|canceling)
        echo "ERROR: Task ${state}:" >&2
        echo "$task_response" | jq '.error' >&2
        exit 1
        ;;
      *)
        printf "    [%d/%d] state=%s\r" "$i" "$max_attempts" "$state"
        sleep .5
        ;;
    esac
  done

  echo "ERROR: Timed out waiting for task after ${max_attempts} attempts." >&2
  exit 1
}

# Looks up a resource by a query and sets RESULT to its pulp_href, or empty.
#   find_href <PATH_WITH_QUERY>
find_href() {
  RESULT=$(pulp_api GET "$1" | jq -r '.results[0].pulp_href // empty')
}

# Ensures a Pulp domain exists, creating it if necessary.
#   ensure_domain <DOMAIN_NAME>
ensure_domain() {
  local domain_name="$1"

  echo "==> Looking up domain '${domain_name}'..."
  find_href "${API_ROOT}/default/api/v3/domains/?name=${domain_name}"
  if [[ -n "$RESULT" ]]; then
    echo "    Domain already exists: ${RESULT}"
    return 0
  fi

  pulp_create "domain" \
    "${API_ROOT}/default/api/v3/domains/" \
    "{
      \"name\": \"${domain_name}\",
      \"storage_class\": \"pulpcore.app.models.storage.FileSystem\",
      \"storage_settings\": {\"location\": \"/var/lib/pulp/${domain_name}/\"},
      \"pulp_labels\": {\"contentsources\": \"true\"}
    }" \
    '.pulp_href'
  echo "    Created domain: ${RESULT}"
}

# Ensures an RPM remote exists. Sets RESULT to its href.
#   ensure_remote <NAME> <URL>
ensure_remote() {
  local name="$1"
  local url="$2"

  find_href "${API_ROOT}/${DOMAIN}/api/v3/remotes/rpm/rpm/?name=${name}"
  if [[ -n "$RESULT" ]]; then
    echo "    Remote already exists: ${RESULT}"
    return 0
  fi
  pulp_create "remote" \
    "${API_ROOT}/${DOMAIN}/api/v3/remotes/rpm/rpm/" \
    "{\"name\": \"${name}\", \"url\": \"${url}\", \"policy\": \"on_demand\"}" \
    '.pulp_href'
  echo "    Created remote: ${RESULT}"
}

# Syncs the repository from a remote if the repository doesn't have the version yet,
# so re-running doesn't add versions.
#   sync_to_version <REPO_HREF> <REMOTE_HREF> <VERSION>
sync_to_version() {
  local repo_href="$1"
  local remote_href="$2"
  local version="$3"

  local latest
  latest=$(pulp_api GET "$repo_href" | jq -r '.latest_version_href | split("/") | .[-2] | tonumber')
  if (( latest >= version )); then
    echo "    Repository already has version ${version}, skipping sync."
    return 0
  fi

  pulp_create "sync" "${repo_href}sync/" "{\"remote\": \"${remote_href}\"}" '.task'
  wait_for_task "$RESULT"

  latest=$(pulp_api GET "$repo_href" | jq -r '.latest_version_href | split("/") | .[-2] | tonumber')
  if (( latest != version )); then
    echo "ERROR: Expected repository version ${version} after sync, got ${latest}." >&2
    exit 1
  fi
  echo "    Synced, repository version ${version}."
}

# Ensures a publication exists for a repository version. Sets RESULT to its href.
#   ensure_publication <VERSION_HREF>
ensure_publication() {
  local version_href="$1"
  local encoded
  encoded=$(jq -nr --arg p "$version_href" '$p|@uri')

  find_href "${API_ROOT}/${DOMAIN}/api/v3/publications/rpm/rpm/?repository_version=${encoded}"
  if [[ -n "$RESULT" ]]; then
    echo "    Publication already exists: ${RESULT}"
    return 0
  fi
  pulp_create "publication" \
    "${API_ROOT}/${DOMAIN}/api/v3/publications/rpm/rpm/" \
    "{\"repository_version\": \"${version_href}\"}" \
    '.task'
  wait_for_task "$RESULT"
  echo "    Created publication: ${RESULT}"
}

# Writes the repository and publication hrefs into a payload file.
#   write_payload <FILE> <REPO_HREF> <PUBLICATION_HREF>
write_payload() {
  local file="$1"
  local repo_href="$2"
  local publication_href="$3"

  local tmp
  tmp="$(mktemp)"
  jq --arg repo "$repo_href" --arg pub "$publication_href" \
    '.data.repository_href = $repo | .data.publication_href = $pub | .time = (now | todate)' "$file" >"$tmp"
  mv "$tmp" "$file"
  echo "    Updated ${file#"${REPO_DIR}/"}"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

ensure_domain "$DOMAIN"

echo "==> Ensuring remotes..."
ensure_remote "${NAME}-build-1" "$BUILD_1_REMOTE_URL"
REMOTE_1_HREF="$RESULT"
ensure_remote "${NAME}-build-2" "$BUILD_2_REMOTE_URL"
REMOTE_2_HREF="$RESULT"

echo "==> Ensuring repository '${NAME}'..."
find_href "${API_ROOT}/${DOMAIN}/api/v3/repositories/rpm/rpm/?name=${NAME}"
if [[ -n "$RESULT" ]]; then
  echo "    Repository already exists: ${RESULT}"
else
  pulp_create "repository" \
    "${API_ROOT}/${DOMAIN}/api/v3/repositories/rpm/rpm/" \
    "{\"name\": \"${NAME}\"}" \
    '.pulp_href'
  echo "    Created repository: ${RESULT}"
fi
REPO_HREF="$RESULT"

echo "==> Build 1: syncing ${BUILD_1_REMOTE_URL}..."
sync_to_version "$REPO_HREF" "$REMOTE_1_HREF" 1
echo "==> Build 2: syncing ${BUILD_2_REMOTE_URL}..."
sync_to_version "$REPO_HREF" "$REMOTE_2_HREF" 2

echo "==> Publishing version 1..."
ensure_publication "${REPO_HREF}versions/1/"
PUBLICATION_1_HREF="$RESULT"
echo "==> Publishing version 2..."
ensure_publication "${REPO_HREF}versions/2/"
PUBLICATION_2_HREF="$RESULT"

echo "==> Ensuring distribution (base_path=${BASE_PATH})..."
find_href "${API_ROOT}/${DOMAIN}/api/v3/distributions/rpm/rpm/?base_path=$(jq -nr --arg p "$BASE_PATH" '$p|@uri')"
if [[ -n "$RESULT" ]]; then
  echo "    Distribution already exists, pointing it at the latest publication: ${RESULT}"
  pulp_create "distribution update" "$RESULT" "{\"publication\": \"${PUBLICATION_2_HREF}\"}" '.task' PATCH
else
  pulp_create "distribution" \
    "${API_ROOT}/${DOMAIN}/api/v3/distributions/rpm/rpm/" \
    "{\"name\": \"${NAME}\", \"base_path\": \"${BASE_PATH}\", \"publication\": \"${PUBLICATION_2_HREF}\"}" \
    '.task'
fi
wait_for_task "$RESULT"

echo "==> Writing hrefs into the payloads..."
write_payload "${PAYLOAD_DIR}/build-end.json" "$REPO_HREF" "$PUBLICATION_1_HREF"
write_payload "${PAYLOAD_DIR}/build-end-second-build.json" "$REPO_HREF" "$PUBLICATION_2_HREF"

echo ""
echo "Done."
echo "  Repository:               ${REPO_HREF}"
echo "  Publication (build 1001): ${PUBLICATION_1_HREF}"
echo "  Publication (build 1006): ${PUBLICATION_2_HREF}"
