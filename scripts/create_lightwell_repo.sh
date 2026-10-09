#!/usr/bin/env bash
#
# Creates Maven and Python repositories in Pulp and imports them into the
# application as lightwell repositories, then loads the local vulnerability,
# advisory, and package-mirror seed onto those repositories.
#
# Reads both lightwell_repos.json and lightwell_demo_repos.json, creates
# the corresponding Pulp remotes, repositories, and distributions under
# the appropriate domains, then runs the Go importer.
#
# Idempotent: existing distributions (matched by base_path) are reused;
# Maven catalog seed (if empty) and the CS import always run. Python
# remediated fixture files are uploaded only if missing from the repository.
# The vulnerability seed is skipped when those rows already exist. Advisories
# and the package mirror are refreshed on every run.
#
# Intended for local development against the Pulp instance started via docker-compose.
# Maven catalogs are seeded with uploaded POM + repository modify (pull-through
# GET does not create MavenPackage units that the packages API lists).
#
# Usage:
#   ./scripts/create_lightwell_repo.sh [--remote-url URL] [--validated-count N]
#
#   --validated-count N  Number of distinct packages to seed into the validated
#                        Maven repo (default 20). Also settable via the
#                        VALIDATED_PACKAGE_COUNT environment variable. Packages
#                        are synthetic POMs uploaded via repository modify so
#                        they appear in the packages API.
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

# Base path of the "validated" Maven repo that should be seeded with a large
# number of distinct packages, and how many packages to seed there.
VALIDATED_BASE_PATH="${VALIDATED_BASE_PATH:-java/validated}"
VALIDATED_PACKAGE_COUNT="${VALIDATED_PACKAGE_COUNT:-20}"

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
    --remote-url)      REMOTE_URL="$2"; shift 2 ;;
    --validated-count) VALIDATED_PACKAGE_COUNT="$2"; shift 2 ;;
    *)                 echo "Unknown option: $1" >&2; exit 1 ;;
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
  local i

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
        sleep .5
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

# Looks up a distribution by base_path. Sets RESULT to pulp_href or empty.
#   find_distribution_by_base_path <DOMAIN_NAME> <DIST_API_TYPE> <BASE_PATH>
find_distribution_by_base_path() {
  local domain_name="$1"
  local dist_api_type="$2"
  local base_path="$3"
  local encoded_base_path
  encoded_base_path=$(jq -nr --arg p "$base_path" '$p|@uri')

  local dist_results
  dist_results=$(pulp_api GET \
    "${API_ROOT}/${domain_name}/api/v3/distributions/${dist_api_type}/?base_path=${encoded_base_path}")
  local dist_count
  dist_count=$(echo "$dist_results" | jq -r '.count // 0')

  if [[ "$dist_count" != "0" && "$dist_count" != "null" ]]; then
    RESULT=$(echo "$dist_results" | jq -r '.results[0].pulp_href')
  else
    RESULT=""
  fi
}

# Ensures a remote, repository, and distribution exist in Pulp for the given
# type. Reuses an existing distribution matched by base_path.
# Sets REPO_HREF to the maven/python repository href.
#   ensure_pulp_repo <DOMAIN_NAME> <TYPE> <BASE_PATH> <REMOTE_URL>
#   TYPE is "maven" or "python"
ensure_pulp_repo() {
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

  echo "==> Looking up ${repo_type} distribution (base_path=${base_path})..."
  find_distribution_by_base_path "$domain_name" "$dist_api_type" "$base_path"
  if [[ -n "$RESULT" ]]; then
    echo "    Distribution already exists: ${RESULT}"
    local dist_json
    dist_json=$(pulp_api GET "$RESULT")
    REPO_HREF=$(echo "$dist_json" | jq -r '.repository // empty')
    if [[ -z "$REPO_HREF" || "$REPO_HREF" == "null" ]]; then
      local dist_name encoded_name repo_results
      dist_name=$(echo "$dist_json" | jq -r '.name')
      encoded_name=$(jq -nr --arg p "$dist_name" '$p|@uri')
      repo_results=$(pulp_api GET \
        "${API_ROOT}/${domain_name}/api/v3/repositories/${api_type}/?name=${encoded_name}")
      REPO_HREF=$(echo "$repo_results" | jq -r '.results[0].pulp_href // empty')
    fi
    if [[ -z "$REPO_HREF" || "$REPO_HREF" == "null" ]]; then
      echo "ERROR: Could not resolve repository href for distribution ${base_path}" >&2
      echo "$dist_json" | jq . >&2
      exit 1
    fi
    echo "    Repository: ${REPO_HREF}"
    return 0
  fi

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
  REPO_HREF="$repo_href"
}

# Uploads a Maven POM and returns the artifact pulp_href in RESULT.
#   upload_maven_pom <DOMAIN_NAME> <RELATIVE_PATH> <POM_BODY>
upload_maven_pom() {
  local domain_name="$1"
  local relative_path="$2"
  local pom_body="$3"

  local tmp
  tmp="$(mktemp)"
  printf '%s' "$pom_body" >"$tmp"

  local -a args
  args=(-s -k -X POST)
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
  args+=(-F "file=@${tmp};filename=$(basename "$relative_path")" -F "relative_path=${relative_path}" -F "overwrite=true")

  local response
  response=$(curl "${args[@]}" "$(pulp_url "${API_ROOT}/${domain_name}/api/v3/content/maven/artifact/")")
  rm -f "$tmp"

  RESULT=$(echo "$response" | jq -r '.pulp_href // empty')
  if [[ -n "$RESULT" && "$RESULT" != "null" ]]; then
    return 0
  fi

  local task_href
  task_href=$(echo "$response" | jq -r '.task // empty')
  if [[ -z "$task_href" || "$task_href" == "null" ]]; then
    echo "ERROR: Failed to upload ${relative_path}:" >&2
    echo "$response" | jq . >&2
    exit 1
  fi
  wait_for_task "$task_href"
  local task_response
  task_response=$(pulp_api GET "$task_href")
  RESULT=$(echo "$task_response" | jq -r '.created_resources[0] // empty')
  if [[ -z "$RESULT" || "$RESULT" == "null" ]]; then
    echo "ERROR: Upload task for ${relative_path} created no resources:" >&2
    echo "$task_response" | jq . >&2
    exit 1
  fi
}

# Seeds MavenPackage units via artifact upload + repository modify.
# Pull-through GETs only create artifacts and leave packages/ empty.
#
# BASE_PATH is used to give each repo a uniquely-namespaced marker package.
# pulp_maven derives a repo's .meta/prefixes.txt from the first two segments of
# each package's group_id; that file's content is deterministic (no timestamp),
# so two repos with the same package set produce a byte-identical prefixes.txt
# and thus the same sha256. MavenMetadata.sha256 is globally unique, but
# pulp_maven's IntegrityError recovery does a domain-scoped lookup, so a second
# domain seeding identical content crashes the modify task with
# "MavenMetadata matching query does not exist". The per-repo marker guarantees
# a unique prefix (hence a unique prefixes.txt) so the collision cannot happen.
#   seed_maven_packages <DOMAIN_NAME> <REPO_HREF> <BASE_PATH>
seed_maven_packages() {
  local domain_name="$1"
  local repo_href="$2"
  local base_path="$3"

  local pkg_json pkg_count
  pkg_json=$(pulp_api GET "${repo_href}packages/?limit=1")
  pkg_count=$(echo "$pkg_json" | jq -r '.count // 0')
  if [[ "$pkg_count" != "0" && "$pkg_count" != "null" ]]; then
    echo "    Maven catalog already has ${pkg_count} package(s); skipping seed."
    return 0
  fi

  echo "    Seeding MavenPackage units (upload POM + modify)..."

  local -a content_hrefs=()

  # Per-repo marker package so this repo's prefixes.txt is globally unique
  # (see function header for why identical prefixes.txt across domains crashes).
  local marker_slug
  marker_slug=$(printf '%s-%s' "$domain_name" "$base_path" | tr -c 'a-zA-Z0-9' '-')
  upload_maven_pom "$domain_name" \
    "csseed/${marker_slug}/marker/1.0.0/marker-1.0.0.pom" \
    "<?xml version=\"1.0\"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>csseed.${marker_slug}</groupId>
  <artifactId>marker</artifactId>
  <version>1.0.0</version>
</project>
"
  content_hrefs+=("$RESULT")

  upload_maven_pom "$domain_name" \
    "blissed/blissed/1.0-beta-3/blissed-1.0-beta-3.pom" \
    '<?xml version="1.0"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>blissed</groupId>
  <artifactId>blissed</artifactId>
  <version>1.0-beta-3</version>
</project>
'
  content_hrefs+=("$RESULT")

  upload_maven_pom "$domain_name" \
    "avalon-util/avalon-util-exception/1.0.0/avalon-util-exception-1.0.0.pom" \
    '<?xml version="1.0"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>avalon-util</groupId>
  <artifactId>avalon-util-exception</artifactId>
  <version>1.0.0</version>
</project>
'
  content_hrefs+=("$RESULT")

  upload_maven_pom "$domain_name" \
    "org/example/demo-lib/5.3.18.rhlw-00001/demo-lib-5.3.18.rhlw-00001.pom" \
    '<?xml version="1.0"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>org.example</groupId>
  <artifactId>demo-lib</artifactId>
  <version>5.3.18.rhlw-00001</version>
</project>
'
  content_hrefs+=("$RESULT")

  upload_maven_pom "$domain_name" \
    "org/example/demo-lib/5.3.18.rhlw-00003/demo-lib-5.3.18.rhlw-00003.pom" \
    '<?xml version="1.0"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>org.example</groupId>
  <artifactId>demo-lib</artifactId>
  <version>5.3.18.rhlw-00003</version>
</project>
'
  content_hrefs+=("$RESULT")

  local body
  body=$(jq -n --args '{add_content_units: $ARGS.positional}' "${content_hrefs[@]}")

  local attempt
  for attempt in 1 2; do
    pulp_create "repository modify" "${repo_href}modify/" "$body" '.task'
    echo "    Waiting for modify task (attempt ${attempt})..."
    if wait_for_task_or_warn "$RESULT"; then
      echo "    Seeded ${#content_hrefs[@]} Maven artifacts."
      return 0
    fi
    sleep .5
  done
  echo "WARN: Maven catalog seed failed for ${repo_href}; packages/ may be empty." >&2
}

# Like wait_for_task but returns 1 on failure instead of exiting.
wait_for_task_or_warn() {
  local task_href="$1"
  local max_attempts=60
  local state=""
  local i

  for ((i = 1; i <= max_attempts; i++)); do
    local task_response
    task_response=$(pulp_api GET "$task_href")
    state=$(echo "$task_response" | jq -r '.state')

    case "$state" in
      completed)
        return 0
        ;;
      failed|canceled|canceling)
        echo "WARN: Task ${state}:" >&2
        echo "$task_response" | jq '.error' >&2
        return 1
        ;;
      *)
        printf "    [%d/%d] state=%s\r" "$i" "$max_attempts" "$state"
        sleep .5
        ;;
    esac
  done

  echo "WARN: Timed out waiting for task after ${max_attempts} attempts." >&2
  return 1
}

# Uploads checked-in wheel and source distributions into the remediated
# Python repository. PyPI cannot supply our remediated versions.
#   populate_python_remediated_repo <DOMAIN_NAME> <BASE_PATH>
populate_python_remediated_repo() (
  local domain_name="$1"
  local base_path="$2"
  local artifact_dir="${REPO_DIR}/scripts/fixtures/python_remediated"

  find_distribution_by_base_path "$domain_name" "python/pypi" "$base_path"
  if [[ -z "$RESULT" ]]; then
    echo "ERROR: Python distribution not found for ${base_path}" >&2
    exit 1
  fi

  local distribution repo_href
  distribution=$(pulp_api GET "$RESULT")
  repo_href=$(echo "$distribution" | jq -er '.repository')

  echo "    Uploading synthetic remediated Python releases..."
  local artifact filename encoded_filename repo_version encoded_version existing response task_href
  for artifact in "$artifact_dir"/*.whl "$artifact_dir"/*.tar.gz; do
    if [[ ! -f "$artifact" ]]; then
      echo "ERROR: Missing remediated Python fixture: ${artifact}" >&2
      exit 1
    fi
    filename=$(basename "$artifact")
    encoded_filename=$(jq -nr --arg value "$filename" '$value|@uri')
    repo_version=$(pulp_api GET "$repo_href" | jq -er '.latest_version_href')
    encoded_version=$(jq -nr --arg value "$repo_version" '$value|@uri')
    existing=$(pulp_api GET \
      "${API_ROOT}/${domain_name}/api/v3/content/python/packages/?filename=${encoded_filename}&repository_version=${encoded_version}" \
      | jq -er '.count')
    if (( existing > 0 )); then
      echo "      Already present: ${filename}"
      continue
    fi

    local -a args=(-sS -k --fail-with-body -F "relative_path=${filename}" -F "repository=${repo_href}" -F "file=@${artifact}")
    if [[ -n "$PULP_CLIENT_CERT" ]]; then
      args+=(--cert "$PULP_CLIENT_CERT")
      if [[ -n "$PULP_CLIENT_KEY" ]]; then
        args+=(--key "$PULP_CLIENT_KEY")
      fi
      if [[ -n "$PULP_CA_CERT" ]]; then
        args+=(--cacert "$PULP_CA_CERT")
      fi
    elif [[ -n "$PULP_USER" ]]; then
      args+=(-u "${PULP_USER}:${PULP_PASS}")
    fi

    response=$(curl "${args[@]}" "$(pulp_url "${API_ROOT}/${domain_name}/api/v3/content/python/packages/")")
    task_href=$(echo "$response" | jq -er '.task')
    wait_for_task "$task_href"
    echo "      Uploaded: ${filename}"
  done
)

# Seeds a large number of *distinct* MavenPackage units into the validated repo.
#
# The application counts a Maven "package" as a distinct group_id:artifact_id
# that has a .pom. This uploads TARGET synthetic POMs, each with a distinct
# artifactId, and adds them to the repository in a single modify so they appear
# in the packages API. Idempotent: if the catalog already holds at least TARGET
# packages the seed is skipped.
#   seed_validated_maven_packages <DOMAIN_NAME> <REPO_HREF> <TARGET_COUNT>
seed_validated_maven_packages() {
  local domain_name="$1"
  local repo_href="$2"
  local target="$3"

  local pkg_json pkg_count
  pkg_json=$(pulp_api GET "${repo_href}packages/?limit=1")
  pkg_count=$(echo "$pkg_json" | jq -r '.count // 0')
  if [[ "$pkg_count" != "null" && "$pkg_count" -ge "$target" ]]; then
    echo "    Validated catalog already has ${pkg_count} package(s) (>= ${target}); skipping seed."
    return 0
  fi

  echo "    Seeding ${target} distinct validated MavenPackage units (upload POM + modify)..."

  local -a content_hrefs=()
  local i artifact_id relative_path
  for ((i = 1; i <= target; i++)); do
    group="org.lightwell.validated"
    artifact_id=$(printf 'validated-pkg-%04d' $RANDOM)
    version="1.0.$RANDOM"
    relative_path="org/lightwell/validated/"${group//./\/}"/${artifact_id}/$version/${artifact_id}-$version.pom"
    upload_maven_pom "$domain_name" "$relative_path" \
      "<?xml version=\"1.0\"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group}</groupId>
  <artifactId>${artifact_id}</artifactId>
  <version>${version}</version>
</project>
"
    content_hrefs+=("$RESULT")
    echo "      ...uploaded ${i}/${target} POMs"
  done

  local body
  body=$(jq -n --args '{add_content_units: $ARGS.positional}' "${content_hrefs[@]}")

  local attempt
  for attempt in 1 2; do
    pulp_create "repository modify" "${repo_href}modify/" "$body" '.task'
    echo "    Waiting for modify task (attempt ${attempt})..."
    if wait_for_task_or_warn "$RESULT"; then
      echo "    Seeded ${#content_hrefs[@]} validated Maven packages."
      return 0
    fi
    sleep 2
  done
  echo "WARN: Validated catalog seed failed for ${repo_href}; packages/ may be incomplete." >&2
}

# Ensures repos from a JSON allowlist file under the given domain.
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
        ensure_pulp_repo "$domain_name" maven "$entry_base_path" "$REMOTE_URL"
        if [[ "$domain_name" == "$DOMAIN" && "$entry_base_path" == "$VALIDATED_BASE_PATH" ]]; then
          seed_validated_maven_packages "$domain_name" "$REPO_HREF" "$VALIDATED_PACKAGE_COUNT"
        else
          seed_maven_packages "$domain_name" "$REPO_HREF" "$entry_base_path"
        fi
        ;;
      python)
        ensure_pulp_repo "$domain_name" python "$entry_base_path" "$PYTHON_REMOTE_URL"
        if [[ "$domain_name" == "$DOMAIN" && "$entry_base_path" == "python/remediated" ]]; then
          populate_python_remediated_repo "$domain_name" "$entry_base_path"
        fi
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

import_err=0
(cd "${REPO_DIR}" && FEATURES_LIGHTWELL_ENABLED=true go run ./cmd/external-repos/main.go import-lightwell-packages) || import_err=$?

if [[ "$import_err" -ne 0 ]]; then
  echo "ERROR: import packages failed (exit code ${import_err})" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Step 4: Seed vulnerabilities, advisories, and the package mirror
# ---------------------------------------------------------------------------
# Pulp still holds the catalog used by the endpoints that read Pulp directly.
# This replaces the mirror rows for the four org -3 repositories so the list
# endpoints match the advisory fixed_versions.

echo ""
echo "==> Seeding lightwell vulnerabilities, advisories, and package mirror..."
"${REPO_DIR}/scripts/seed_lightwell_dev.sh"

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo ""
echo "Done! Repositories created in Pulp, imported into the application, and seeded."
echo ""
echo "  Auth mode:    $(if [[ -n "$PULP_CLIENT_CERT" ]]; then echo "cert"; else echo "basic"; fi)"
echo "  Domain:       ${DOMAIN} ($(jq length "${LIGHTWELL_JSON}") repos)"
echo "  Demo domain:  ${DEMO_DOMAIN} ($(jq length "${LIGHTWELL_DEMO_JSON}") repos)"
