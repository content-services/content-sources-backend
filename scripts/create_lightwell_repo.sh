#!/bin/bash
#
# Creates Maven and Python repositories in Pulp and imports them into the
# application as lightwell repositories.
#
# Reads both lightwell_repos.json and lightwell_demo_repos.json, creates
# the corresponding Pulp remotes, repositories, and distributions under
# the appropriate domains, then runs the Go importer.
#
# Intended for local development against the Pulp instance started via docker-compose.
#
# Usage:
#   ./scripts/create_maven_repo.sh [--remote-url URL]
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
PULP_CONTENT_URL="${PULP_CONTENT_URL:-http://localhost:8081}"
DOMAIN="${DOMAIN:-lightwell}"
DEMO_DOMAIN="${DEMO_DOMAIN:-public-lightwell-demo}"
REMOTE_URL="${REMOTE_URL:-https://repo.maven.apache.org/maven2/}"
PYTHON_REMOTE_URL="${PYTHON_REMOTE_URL:-https://pypi.org/simple/}"
API_ROOT="/api/pulp"
SUFFIX="$(date +%s)"
REPO_DIR="$(cd "$(dirname "$0")/.."; pwd)"
LIGHTWELL_JSON="${REPO_DIR}/pkg/external_repos/lightwell_repos.json"
LIGHTWELL_DEMO_JSON="${REPO_DIR}/pkg/external_repos/lightwell_demo_repos.json"

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
    --remote-url) REMOTE_URL="$2"; shift 2 ;;
    *)            echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Calls the Pulp REST API and returns the JSON response body.
# Uses cert auth when PULP_CLIENT_CERT is set, otherwise basic auth.
#   pulp_api <METHOD> <PATH> [BODY]
pulp_api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"

  local -a args=(
    -s -k
    -X "${method}"
    -H "Content-Type: application/json"
  )

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

# Ensures a Pulp domain exists, creating it if necessary.
#   ensure_domain <DOMAIN_NAME>
ensure_domain() {
  local domain_name="$1"

  echo "==> Looking up domain '${domain_name}'..."

  local domain_results
  domain_results=$(pulp_api GET "${API_ROOT}/default/api/v3/domains/?name=${domain_name}")
  local domain_count
  domain_count=$(echo "$domain_results" | jq -r '.count')

  if [[ "$domain_count" == "0" ]]; then
    echo "    Domain not found, creating..."
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
  else
    RESULT=$(echo "$domain_results" | jq -r '.results[0].pulp_href')
    echo "    Domain already exists: ${RESULT}"
  fi
}

# Creates a remote, repository, and distribution in Pulp for the given type.
#   create_pulp_repo <DOMAIN_NAME> <TYPE> <BASE_PATH> <REMOTE_URL>
#   TYPE is "maven" or "python"
create_pulp_repo() {
  local domain_name="$1"
  local repo_type="$2"
  local base_path="$3"
  local url="$4"

  local api_type dist_api_type
  case "$repo_type" in
    maven)  api_type="maven/maven";   dist_api_type="maven/maven" ;;
    python) api_type="python/python"; dist_api_type="python/pypi" ;;
    *)      echo "ERROR: Unsupported repo type: ${repo_type}" >&2; exit 1 ;;
  esac

  local name_slug
  name_slug=$(echo "$base_path" | tr '/' '-')

  echo "==> Creating ${repo_type} remote -> ${url}..."
  pulp_create "remote" \
    "${API_ROOT}/${domain_name}/api/v3/remotes/${api_type}/" \
    "{\"name\": \"${name_slug}-remote-${SUFFIX}\", \"url\": \"${url}\"}" \
    '.pulp_href'
  local remote_href="$RESULT"
  echo "    Remote: ${remote_href}"

  echo "==> Creating ${repo_type} repository..."
  pulp_create "repository" \
    "${API_ROOT}/${domain_name}/api/v3/repositories/${api_type}/" \
    "{\"name\": \"${name_slug}-repo-${SUFFIX}\", \"remote\": \"${remote_href}\"}" \
    '.pulp_href'
  local repo_href="$RESULT"
  echo "    Repository: ${repo_href}"

  echo "==> Creating ${repo_type} distribution (base_path=${base_path})..."
  pulp_create "distribution" \
    "${API_ROOT}/${domain_name}/api/v3/distributions/${dist_api_type}/" \
    "{
      \"name\": \"${base_path}\",
      \"base_path\": \"${base_path}\",
      \"repository\": \"${repo_href}\",
      \"remote\": \"${remote_href}\"
    }" \
    '.task'
  local task_href="$RESULT"

  echo "==> Waiting for distribution task to complete..."
  wait_for_task "$task_href"
  echo "    Distribution created successfully."
}

# Pool of Maven packages that can be fetched through a distribution to
# populate the Pulp cache.
MAVEN_PACKAGES=(
  /blissed/blissed/1.0-beta-3/blissed-1.0-beta-3.pom
  /blissed/blissed/1.0-beta-3/blissed-1.0-beta-3.jar
  /avalon-util/avalon-util-exception/1.0.0/avalon-util-exception-1.0.0.pom
  /avalon-util/avalon-util-exception/1.0.0/avalon-util-exception-1.0.0.jar
  /commons-logging/commons-logging/1.0.4/commons-logging-1.0.4.pom
  /commons-logging/commons-logging/1.0.4/commons-logging-1.0.4.jar
  /commons-io/commons-io/2.11.0/commons-io-2.11.0.pom
  /commons-io/commons-io/2.11.0/commons-io-2.11.0.jar
  /commons-codec/commons-codec/1.15/commons-codec-1.15.pom
  /commons-codec/commons-codec/1.15/commons-codec-1.15.jar
  /commons-lang/commons-lang/2.6/commons-lang-2.6.pom
  /commons-lang/commons-lang/2.6/commons-lang-2.6.jar
  /commons-collections/commons-collections/3.2.2/commons-collections-3.2.2.pom
  /commons-collections/commons-collections/3.2.2/commons-collections-3.2.2.jar
  /junit/junit/4.13.2/junit-4.13.2.pom
  /junit/junit/4.13.2/junit-4.13.2.jar
  /org/slf4j/slf4j-api/1.7.36/slf4j-api-1.7.36.pom
  /org/slf4j/slf4j-api/1.7.36/slf4j-api-1.7.36.jar
  /org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.pom
  /org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar
  /com/google/guava/guava/31.1-jre/guava-31.1-jre.pom
  /com/google/guava/guava/31.1-jre/guava-31.1-jre.jar
  /com/google/code/gson/gson/2.10.1/gson-2.10.1.pom
  /com/google/code/gson/gson/2.10.1/gson-2.10.1.jar
  /org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.pom
  /org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar
  /org/apache/commons/commons-text/1.10.0/commons-text-1.10.0.pom
  /org/apache/commons/commons-text/1.10.0/commons-text-1.10.0.jar
  /org/apache/httpcomponents/httpclient/4.5.14/httpclient-4.5.14.pom
  /org/apache/httpcomponents/httpclient/4.5.14/httpclient-4.5.14.jar
  /org/apache/httpcomponents/httpcore/4.4.16/httpcore-4.4.16.pom
  /org/apache/httpcomponents/httpcore/4.4.16/httpcore-4.4.16.jar
  /org/yaml/snakeyaml/1.33/snakeyaml-1.33.pom
  /org/yaml/snakeyaml/1.33/snakeyaml-1.33.jar
  /com/fasterxml/jackson/core/jackson-core/2.15.2/jackson-core-2.15.2.pom
  /com/fasterxml/jackson/core/jackson-core/2.15.2/jackson-core-2.15.2.jar
  /com/fasterxml/jackson/core/jackson-databind/2.15.2/jackson-databind-2.15.2.pom
  /com/fasterxml/jackson/core/jackson-databind/2.15.2/jackson-databind-2.15.2.jar
  /org/mockito/mockito-core/5.3.1/mockito-core-5.3.1.pom
  /org/mockito/mockito-core/5.3.1/mockito-core-5.3.1.jar
)

# Shuffle the pool once; each call to populate_maven_repo deals from the top.
MAVEN_DEAL_INDEX=0

# Fetches 1-4 Maven packages through a distribution to pull content into the
# repo via its on-demand remote. Deals from a pre-shuffled pool so each repo
# gets distinct packages.
#   populate_maven_repo <DOMAIN_NAME> <BASE_PATH>
populate_maven_repo() {
  readarray -t SHUFFLED_MAVEN_PACKAGES < <(printf '%s\n' "${MAVEN_PACKAGES[@]}" | shuf)
  local domain_name="$1"
  local base_path="$2"
  local content_url="${PULP_CONTENT_URL}/api/pulp-content/${domain_name}/${base_path}"

  local total=${#SHUFFLED_MAVEN_PACKAGES[@]}
  local count=$(( (RANDOM % 8) + 1 ))

  # Wrap around if we've dealt most of the deck
  if (( MAVEN_DEAL_INDEX + count > total )); then
    MAVEN_DEAL_INDEX=0
  fi

  local -a selected=("${SHUFFLED_MAVEN_PACKAGES[@]:MAVEN_DEAL_INDEX:count}")
  MAVEN_DEAL_INDEX=$(( MAVEN_DEAL_INDEX + count ))

  echo "    Fetching ${count} package(s) to populate ${base_path}..."

  local success=0
  local failed=0
  for pkg in "${selected[@]}"; do
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" "${content_url}${pkg}")
    local name
    name=$(basename "$pkg")
    if [[ "$status" -ge 200 && "$status" -lt 400 ]]; then
      success=$((success + 1))
      echo "      OK: ${name}"
    else
      failed=$((failed + 1))
      echo "      WARN: ${name} returned HTTP ${status}"
    fi
  done
  echo "    Fetched ${success}/${count} packages successfully."
}

# Creates repos from a JSON allowlist file under the given domain.
#   create_repos_from_json <DOMAIN_NAME> <JSON_FILE>
create_repos_from_json() {
  local domain_name="$1"
  local json_file="$2"

  local repo_count
  repo_count=$(jq length "${json_file}")
  echo "    Found ${repo_count} repo(s) in $(basename "${json_file}")."

  for idx in $(seq 0 $((repo_count - 1))); do
    local entry_name entry_type entry_base_path
    entry_name=$(jq -r ".[$idx].name" "${json_file}")
    entry_type=$(jq -r ".[$idx].type" "${json_file}")
    entry_base_path=$(jq -r ".[$idx].base_path" "${json_file}")

    echo ""
    echo "--- [$(( idx + 1 ))/${repo_count}] ${entry_name} (${entry_type}) ---"

    case "$entry_type" in
      maven)
        create_pulp_repo "$domain_name" maven "$entry_base_path" "$REMOTE_URL"
        populate_maven_repo "$domain_name" "$entry_base_path"
        ;;
      python)
        create_pulp_repo "$domain_name" python "$entry_base_path" "$PYTHON_REMOTE_URL"
        ;;
      *)
        echo "WARN: Skipping unsupported type '${entry_type}' for ${entry_name}"
        continue
        ;;
    esac
  done
}

# ---------------------------------------------------------------------------
# Step 1: Create lightwell repos under the "lightwell" domain
# ---------------------------------------------------------------------------

ensure_domain "$DOMAIN"

echo ""
echo "==> Creating lightwell repos under domain '${DOMAIN}'..."
create_repos_from_json "$DOMAIN" "$LIGHTWELL_JSON"

# ---------------------------------------------------------------------------
# Step 2: Create demo repos under the "public-lightwell-demo" domain
# ---------------------------------------------------------------------------

echo ""
ensure_domain "$DEMO_DOMAIN"

echo ""
echo "==> Creating demo repos under domain '${DEMO_DOMAIN}'..."
create_repos_from_json "$DEMO_DOMAIN" "$LIGHTWELL_DEMO_JSON"

# ---------------------------------------------------------------------------
# Step 3: Import all repos via the application's lightwell import
# ---------------------------------------------------------------------------

echo ""
echo "==> Importing repositories into the application..."

import_err=0
(cd "${REPO_DIR}" && FEATURES_LIGHTWELL_ENABLED=true go run ./cmd/external-repos/main.go import) || import_err=$?

if [[ "$import_err" -ne 0 ]]; then
  echo "ERROR: import failed (exit code ${import_err})" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo ""
echo "Done! Repositories created in Pulp and imported into the application."
echo ""
echo "  Auth mode:    $(if [[ -n "$PULP_CLIENT_CERT" ]]; then echo "cert"; else echo "basic"; fi)"
echo "  Domain:       ${DOMAIN} ($(jq length "${LIGHTWELL_JSON}") repos)"
echo "  Demo domain:  ${DEMO_DOMAIN} ($(jq length "${LIGHTWELL_DEMO_JSON}") repos)"
