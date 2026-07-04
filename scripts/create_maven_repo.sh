#!/bin/bash
#
# Creates a Maven repository in Pulp backed by Maven Central, then imports it
# into the application as a lightwell repository.
#
# Intended for local development against the Pulp instance started via docker-compose.
#
# Usage:
#   ./scripts/create_maven_repo.sh [--domain DOMAIN] [--remote-url URL]
#
# Defaults:
#   DOMAIN     = lightwell
#   REMOTE_URL = https://repo.maven.apache.org/maven2/

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration (override with environment variables)
# ---------------------------------------------------------------------------

PULP_URL="${PULP_URL:-https://localhost:8443}"
PULP_USER="${PULP_USER:-admin}"
PULP_PASS="${PULP_PASS:-password}"
PULP_CONTENT_URL="${PULP_CONTENT_URL:-http://localhost:8081}"
DOMAIN="${DOMAIN:-lightwell}"
REMOTE_URL="${REMOTE_URL:-https://repo.maven.apache.org/maven2/}"
API_ROOT="/api/pulp"
SUFFIX="$(date +%s)"
REPO_DIR="$(cd "$(dirname "$0")/.."; pwd)"
LIGHTWELL_JSON="${REPO_DIR}/pkg/external_repos/lightwell_repos.json"

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)     DOMAIN="$2";     shift 2 ;;
    --remote-url) REMOTE_URL="$2"; shift 2 ;;
    *)            echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Calls the Pulp REST API and returns the JSON response body.
#   pulp_api <METHOD> <PATH> [BODY]
pulp_api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"

  local -a args=(
    -s -k
    -u "${PULP_USER}:${PULP_PASS}"
    -X "${method}"
    -H "Content-Type: application/json"
  )
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi

  curl "${args[@]}" "${PULP_URL}${path}"
}

# Calls pulp_api to POST a resource, extracts a field from the response, and
# exits with a clear error if the field is missing.
#   pulp_create <LABEL> <PATH> <BODY> <JQ_FIELD>
#   Prints the extracted value and sets RESULT.
pulp_create() {
  local label="$1"
  local path="$2"
  local body="$3"
  local field="$4"

  local response
  response=$(pulp_api POST "$path" "$body")

  RESULT=$(echo "$response" | jq -r "$field")

  if [[ "$RESULT" == "null" ]]; then
    echo "ERROR: Failed to create ${label}:" >&2
    echo "$response" | jq . >&2
    exit 1
  fi
}

# Polls a Pulp task until it reaches a terminal state.
#   wait_for_task <TASK_HREF>
wait_for_task() {
  local task_href="$1"
  local max_attempts=60
  local state=""

  for ((i = 1; i <= max_attempts; i++)); do
    local task_response
    task_response=$(pulp_api GET "$task_href")
    state=$(echo "$task_response" | jq -r '.state')

    case "$state" in
      completed)
        return 0
        ;;
      failed|canceled|canceling)
        echo "ERROR: Task ${state}:" >&2
        echo "$task_response" | jq '.error' >&2
        exit 1
        ;;
      *)
        printf "    [%d/%d] state=%s\r" "$i" "$max_attempts" "$state"
        sleep 2
        ;;
    esac
  done

  echo "ERROR: Timed out waiting for task after ${max_attempts} attempts." >&2
  exit 1
}

# ---------------------------------------------------------------------------
# Step 1: Look up or create the Pulp domain
# ---------------------------------------------------------------------------

echo "==> Looking up domain '${DOMAIN}'..."

domain_results=$(pulp_api GET "${API_ROOT}/default/api/v3/domains/?name=${DOMAIN}")
domain_count=$(echo "$domain_results" | jq -r '.count')

if [[ "$domain_count" == "0" ]]; then
  echo "    Domain not found, creating..."
  pulp_create "domain" \
    "${API_ROOT}/default/api/v3/domains/" \
    "{
      \"name\": \"${DOMAIN}\",
      \"storage_class\": \"pulpcore.app.models.storage.FileSystem\",
      \"storage_settings\": {\"location\": \"/var/lib/pulp/${DOMAIN}/\"},
      \"pulp_labels\": {\"contentsources\": \"true\"}
    }" \
    '.pulp_href'
  echo "    Created domain: ${RESULT}"
else
  RESULT=$(echo "$domain_results" | jq -r '.results[0].pulp_href')
  echo "    Domain already exists: ${RESULT}"
fi

# ---------------------------------------------------------------------------
# Step 2: Create Maven remote -> repository -> distribution
# ---------------------------------------------------------------------------

echo "==> Creating Maven remote -> ${REMOTE_URL}..."
pulp_create "remote" \
  "${API_ROOT}/${DOMAIN}/api/v3/remotes/maven/maven/" \
  "{\"name\": \"maven-remote-${SUFFIX}\", \"url\": \"${REMOTE_URL}\"}" \
  '.pulp_href'
remote_href="$RESULT"
echo "    Remote: ${remote_href}"

echo "==> Creating Maven repository..."
pulp_create "repository" \
  "${API_ROOT}/${DOMAIN}/api/v3/repositories/maven/maven/" \
  "{\"name\": \"maven-repo-${SUFFIX}\", \"remote\": \"${remote_href}\"}" \
  '.pulp_href'
repo_href="$RESULT"
echo "    Repository: ${repo_href}"

dist_path="java/dev-${SUFFIX}"
echo "==> Creating Maven distribution (base_path=${dist_path})..."
pulp_create "distribution" \
  "${API_ROOT}/${DOMAIN}/api/v3/distributions/maven/maven/" \
  "{
    \"name\": \"${dist_path}\",
    \"base_path\": \"${dist_path}\",
    \"repository\": \"${repo_href}\",
    \"remote\": \"${remote_href}\"
  }" \
  '.task'
task_href="$RESULT"

echo "==> Waiting for distribution task to complete..."
wait_for_task "$task_href"
echo "    Distribution created successfully."

# ---------------------------------------------------------------------------
# Step 3: Import the repo via the application's lightwell import
#
# lightwell_repos.json is compiled into the Go binary via go:embed, so we
# temporarily replace it with a single entry pointing at the distribution we
# just created, run `go run` (which recompiles), then restore the original.
# ---------------------------------------------------------------------------

echo ""
echo "==> Importing repository into the application..."

cp "${LIGHTWELL_JSON}" "${LIGHTWELL_JSON}.bak"
trap 'mv -f "${LIGHTWELL_JSON}.bak" "${LIGHTWELL_JSON}" 2>/dev/null; echo "    Restored lightwell_repos.json"' EXIT

cat > "${LIGHTWELL_JSON}" <<JSONEOF
[
  {
    "name": "lightwell/java/dev-${SUFFIX}",
    "type": "maven",
    "base_path": "${dist_path}",
    "feature_name": "lightwell-network"
  }
]
JSONEOF

import_err=0
(cd "${REPO_DIR}" && FEATURES_LIGHTWELL_ENABLED=true go run ./cmd/external-repos/main.go import) || import_err=$?

if [[ "$import_err" -ne 0 ]]; then
  echo "ERROR: import failed (exit code ${import_err})" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Step 4: Fetch packages through the distribution to populate the Pulp cache
# ---------------------------------------------------------------------------

content_url="${PULP_CONTENT_URL}/api/pulp-content/${DOMAIN}/${dist_path}"

packages=(
  /blissed/blissed/1.0-beta-3/blissed-1.0-beta-3.pom
  /avalon-util/avalon-util-exception/1.0.0/avalon-util-exception-1.0.0.pom
  /commons-logging/commons-logging/1.0.4/commons-logging-1.0.4.pom
  /commons-io/commons-io/2.11.0/commons-io-2.11.0.pom
  /commons-codec/commons-codec/1.15/commons-codec-1.15.pom
  /commons-lang/commons-lang/2.6/commons-lang-2.6.pom
  /commons-collections/commons-collections/3.2.2/commons-collections-3.2.2.pom
  /junit/junit/4.13.2/junit-4.13.2.pom
  /org/slf4j/slf4j-api/1.7.36/slf4j-api-1.7.36.pom
  /org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.pom
  /com/google/guava/guava/31.1-jre/guava-31.1-jre.pom
  /com/google/code/gson/gson/2.10.1/gson-2.10.1.pom
  /org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.pom
  /org/apache/commons/commons-text/1.10.0/commons-text-1.10.0.pom
  /org/apache/httpcomponents/httpclient/4.5.14/httpclient-4.5.14.pom
  /org/apache/httpcomponents/httpcore/4.4.16/httpcore-4.4.16.pom
  /org/yaml/snakeyaml/1.33/snakeyaml-1.33.pom
  /com/fasterxml/jackson/core/jackson-core/2.15.2/jackson-core-2.15.2.pom
  /com/fasterxml/jackson/core/jackson-databind/2.15.2/jackson-databind-2.15.2.pom
  /org/mockito/mockito-core/5.3.1/mockito-core-5.3.1.pom
)

echo ""
echo "==> Fetching ${#packages[@]} packages to populate the Pulp cache..."

success=0
failed=0
for pkg in "${packages[@]}"; do
  status=$(curl -s -o /dev/null -w "%{http_code}" "${content_url}${pkg}")
  name=$(basename "$pkg")
  if [[ "$status" -ge 200 && "$status" -lt 400 ]]; then
    success=$((success + 1))
  else
    failed=$((failed + 1))
    echo "    WARN: ${name} returned HTTP ${status}"
  fi
  printf "    [%d/%d] fetched\r" "$((success + failed))" "${#packages[@]}"
done
echo "    Fetched ${success}/${#packages[@]} packages successfully.    "

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo ""
echo "Done! Maven repository created in Pulp and imported into the application."
echo ""
echo "  Domain:       ${DOMAIN}"
echo "  Remote:       ${remote_href}"
echo "  Repository:   ${repo_href}"
echo "  Distribution: ${dist_path}"
echo "  Content URL:  ${content_url}"
echo "  Packages:     ${success} cached"
