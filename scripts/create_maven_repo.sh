#!/bin/bash
#
# Creates Maven repositories in Pulp and imports them into the application as
# lightwell repositories.
#
# By default, creates two repositories backed by static fixture data that
# mirrors the real lightwell structure:
#   - maven-upstream  (base_path: java/validated)
#   - maven-releases  (base_path: java/remediated)
#
# With --seed-from-maven-central, creates a single repository backed by Maven
# Central with a unique suffix. This mode produces more data but is less
# representative of real data. Multiple repos can be created this way.
#
# Intended for local development against the Pulp instance started via
# docker-compose.
#
# Usage:
#   ./scripts/create_maven_repo.sh [--domain DOMAIN]
#   ./scripts/create_maven_repo.sh --seed-from-maven-central [--remote-url URL]
#

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

SEED_FROM_MAVEN_CENTRAL=false
 diff
FIXTURE_UPSTREAM_URL="https://content-services.github.io/fixtures/maven/maven-upstream/"
FIXTURE_RELEASES_URL="https://content-services.github.io/fixtures/maven/maven-releases/"

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)                    DOMAIN="$2";              shift 2 ;;
    --remote-url)                REMOTE_URL="$2";          shift 2 ;;
    --seed-from-maven-central)   SEED_FROM_MAVEN_CENTRAL=true; shift ;;
    *)                           echo "Unknown option: $1" >&2; exit 1 ;;
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

# Creates a Maven remote, repository, and distribution in Pulp.
#   create_pulp_repo <REMOTE_URL> <BASE_PATH> <LABEL>
# Sets: remote_href, repo_href
create_pulp_repo() {
  local url="$1"
  local base_path="$2"
  local label="$3"

  echo "==> Creating Maven remote for ${label} -> ${url}..."
  pulp_create "remote" \
    "${API_ROOT}/${DOMAIN}/api/v3/remotes/maven/maven/" \
    "{\"name\": \"${label}-remote-${SUFFIX}\", \"url\": \"${url}\"}" \
    '.pulp_href'
  remote_href="$RESULT"
  echo "    Remote: ${remote_href}"

  echo "==> Creating Maven repository for ${label}..."
  pulp_create "repository" \
    "${API_ROOT}/${DOMAIN}/api/v3/repositories/maven/maven/" \
    "{\"name\": \"${label}-repo-${SUFFIX}\", \"remote\": \"${remote_href}\"}" \
    '.pulp_href'
  repo_href="$RESULT"
  echo "    Repository: ${repo_href}"

  echo "==> Creating Maven distribution (base_path=${base_path})..."
  pulp_create "distribution" \
    "${API_ROOT}/${DOMAIN}/api/v3/distributions/maven/maven/" \
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

# Fetches a list of packages through a distribution to populate the Pulp cache.
#   fetch_packages <CONTENT_URL> <PACKAGES_ARRAY_NAME>
fetch_packages() {
  local content_url="$1"
  local -n pkgs=$2

  echo ""
  echo "==> Fetching ${#pkgs[@]} packages to populate the Pulp cache..."

  local success=0
  local failed=0
  for pkg in "${pkgs[@]}"; do
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" "${content_url}${pkg}")
    local name
    name=$(basename "$pkg")
    if [[ "$status" -ge 200 && "$status" -lt 400 ]]; then
      success=$((success + 1))
    else
      failed=$((failed + 1))
      echo "    WARN: ${name} returned HTTP ${status}"
    fi
    printf "    [%d/%d] fetched\r" "$((success + failed))" "${#pkgs[@]}"
  done
  echo "    Fetched ${success}/${#pkgs[@]} packages successfully.    "
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
# Step 2 & 3: Create repos and import (mode-dependent)
# ---------------------------------------------------------------------------

if [[ "$SEED_FROM_MAVEN_CENTRAL" == "true" ]]; then
  # -------------------------------------------------------------------------
  # Maven Central mode: single repo with dynamic suffix (original behavior)
  # -------------------------------------------------------------------------

  dist_path="java/dev-${SUFFIX}"
  create_pulp_repo "$REMOTE_URL" "$dist_path" "maven"

  # Import the repo via the application's lightwell import
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

  # Fetch packages to populate the Pulp cache
  content_url="${PULP_CONTENT_URL}/api/pulp-content/${DOMAIN}/${dist_path}"

  central_packages=(
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

  fetch_packages "$content_url" central_packages

  echo ""
  echo "Done! Maven repository created in Pulp and imported into the application."
  echo ""
  echo "  Domain:       ${DOMAIN}"
  echo "  Distribution: ${dist_path}"
  echo "  Content URL:  ${content_url}"

else
  # -------------------------------------------------------------------------
  # Fixture mode (default): two repos with curated fixture data
  # -------------------------------------------------------------------------

  create_pulp_repo "$FIXTURE_UPSTREAM_URL" "java/validated" "maven-upstream"
  upstream_repo_href="$repo_href"
  upstream_remote_href="$remote_href"

  create_pulp_repo "$FIXTURE_RELEASES_URL" "java/remediated" "maven-releases"
  releases_repo_href="$repo_href"
  releases_remote_href="$remote_href"

  # Import both repos via the application's lightwell import
  echo ""
  echo "==> Importing repositories into the application..."

  cp "${LIGHTWELL_JSON}" "${LIGHTWELL_JSON}.bak"
  trap 'mv -f "${LIGHTWELL_JSON}.bak" "${LIGHTWELL_JSON}" 2>/dev/null; echo "    Restored lightwell_repos.json"' EXIT

  cat > "${LIGHTWELL_JSON}" <<JSONEOF
[
  {
    "name": "lightwell/java/validated",
    "type": "maven",
    "base_path": "java/validated",
    "feature_name": "lightwell-network"
  },
  {
    "name": "lightwell/java/remediated",
    "type": "maven",
    "base_path": "java/remediated",
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

  # Fetch packages to populate the Pulp cache for both repos
  upstream_content_url="${PULP_CONTENT_URL}/api/pulp-content/${DOMAIN}/java/validated"
  releases_content_url="${PULP_CONTENT_URL}/api/pulp-content/${DOMAIN}/java/remediated"

  upstream_packages=(
    /blissed/blissed/1.0-beta-3/blissed.blissed.1.0-beta-3.pom
    /blissed/blissed/1.0-beta-3/blissed.blissed.1.0-beta-3.jar
    /com/example/fixture/raccoon/1.0.0/com.example.fixture.raccoon.1.0.0.pom
    /com/example/fixture/raccoon/1.0.0/com.example.fixture.raccoon.1.0.0.jar
    /com/example/fixture/raccoon/1.1.0/com.example.fixture.raccoon.1.1.0.pom
    /com/example/fixture/raccoon/1.1.0/com.example.fixture.raccoon.1.1.0.jar
    /com/example/fixture/raccoon/2.0.0/com.example.fixture.raccoon.2.0.0.pom
    /com/example/fixture/raccoon/2.0.0/com.example.fixture.raccoon.2.0.0.jar
    /com/example/fixture/raccoon/3.0.0/com.example.fixture.raccoon.3.0.0.pom
    /com/example/fixture/raccoon/3.0.0/com.example.fixture.raccoon.3.0.0.jar
    /org/yaml/snakeyaml/1.33/org.yaml.snakeyaml.1.33.pom
    /org/yaml/snakeyaml/1.33/org.yaml.snakeyaml.1.33.jar
  )

  releases_packages=(
    /blissed/blissed/1.0-beta-3.rhlw-00001/blissed.blissed.1.0-beta-3.rhlw-00001.pom
    /blissed/blissed/1.0-beta-3.rhlw-00001/blissed.blissed.1.0-beta-3.rhlw-00001.jar
    /blissed/blissed/1.0-beta-3.rhlw-00002/blissed.blissed.1.0-beta-3.rhlw-00002.pom
    /blissed/blissed/1.0-beta-3.rhlw-00002/blissed.blissed.1.0-beta-3.rhlw-00002.jar
    /com/example/fixture/raccoon/1.0.0.rhlw-00001/com.example.fixture.raccoon.1.0.0.rhlw-00001.pom
    /com/example/fixture/raccoon/1.0.0.rhlw-00001/com.example.fixture.raccoon.1.0.0.rhlw-00001.jar
    /com/example/fixture/raccoon/1.0.0.rhlw-00002/com.example.fixture.raccoon.1.0.0.rhlw-00002.pom
    /com/example/fixture/raccoon/1.0.0.rhlw-00002/com.example.fixture.raccoon.1.0.0.rhlw-00002.jar
    /com/example/fixture/raccoon/2.0.0.rhlw-00001/com.example.fixture.raccoon.2.0.0.rhlw-00001.pom
    /com/example/fixture/raccoon/2.0.0.rhlw-00001/com.example.fixture.raccoon.2.0.0.rhlw-00001.jar
    /com/example/fixture/raccoon/2.0.0.rhlw-00002/com.example.fixture.raccoon.2.0.0.rhlw-00002.pom
    /com/example/fixture/raccoon/2.0.0.rhlw-00002/com.example.fixture.raccoon.2.0.0.rhlw-00002.jar
    /org/yaml/snakeyaml/1.33.rhlw-00001/org.yaml.snakeyaml.1.33.rhlw-00001.pom
    /org/yaml/snakeyaml/1.33.rhlw-00001/org.yaml.snakeyaml.1.33.rhlw-00001.jar
  )

  fetch_packages "$upstream_content_url" upstream_packages
  fetch_packages "$releases_content_url" releases_packages

  echo ""
  echo "Done! Fixture Maven repositories created in Pulp and imported into the application."
  echo ""
  echo "  Domain:       ${DOMAIN}"
  echo "  Upstream:     java/validated  -> ${FIXTURE_UPSTREAM_URL}"
  echo "  Releases:     java/remediated -> ${FIXTURE_RELEASES_URL}"
  echo "  Content URLs:"
  echo "    ${upstream_content_url}"
  echo "    ${releases_content_url}"
fi
