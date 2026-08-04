#!/usr/bin/env bash
#
# Smoke test matrix for Lightwell access tokens (CS API + optional Pulp content).
#
# Prerequisites:
#   - make run (API on CS_BASE)
#   - features.lightwell.enabled: true
#   - lightwell-network in options.feature_filter when entitle_all is true
#   - seeded Lightwell repo (REPO_UUID) with packages
#
# Usage:
#   REPO_UUID=<uuid> ./scripts/smoke_lightwell_tokens.sh
#   CONTENT_URL=http://localhost:8081/api/pulp-content/lightwell/java/remediated/junit/junit/4.13.2/junit-4.13.2.pom \
#     REPO_UUID=<uuid> ./scripts/smoke_lightwell_tokens.sh
#   EXPECT_PULP_GUARDED=1 CONTENT_URL=... REPO_UUID=... ./scripts/smoke_lightwell_tokens.sh
#
set -euo pipefail

CS_BASE="${CS_BASE:-http://localhost:8000}"
ORG_ID="${ORG_ID:-9999}"
USER_ID="${USER_ID:-1111}"
REPO_UUID="${REPO_UUID:-}"
CONTENT_URL="${CONTENT_URL:-}"
SERVICE_AUTH="${SERVICE_AUTH:-local-dev-lightwell-validate-secret}"
EXPECT_PULP_GUARDED="${EXPECT_PULP_GUARDED:-0}"
API_PREFIX="${API_PREFIX:-/api/content-sources/v1.0}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0
RESULTS=()

header_user() {
  # shellcheck disable=SC1091
  "${SCRIPT_DIR}/header.sh" "${ORG_ID}" "${USER_ID}"
}

header_admin() {
  "${SCRIPT_DIR}/header_org_admin.sh" "${ORG_ID}" "${USER_ID}"
}

expect_status() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [[ "${actual}" == "${expected}" ]]; then
    RESULTS+=("PASS  ${name} (HTTP ${actual})")
    PASS=$((PASS + 1))
  else
    RESULTS+=("FAIL  ${name} (expected ${expected}, got ${actual})")
    FAIL=$((FAIL + 1))
  fi
}

expect_status_one_of() {
  local name="$1"
  local actual="$2"
  shift 2
  local expected
  for expected in "$@"; do
    if [[ "${actual}" == "${expected}" ]]; then
      RESULTS+=("PASS  ${name} (HTTP ${actual})")
      PASS=$((PASS + 1))
      return
    fi
  done
  RESULTS+=("FAIL  ${name} (expected one of [$*], got ${actual})")
  FAIL=$((FAIL + 1))
}

http_code() {
  local method="$1"
  local url="$2"
  shift 2
  curl -s -o /dev/null -w '%{http_code}' -X "${method}" "$@" "${url}"
}

if [[ -z "${REPO_UUID}" ]]; then
  echo "ERROR: REPO_UUID is required" >&2
  exit 1
fi

PACKAGES_URL="${CS_BASE}${API_PREFIX}/repositories/${REPO_UUID}/packages"
TOKENS_URL="${CS_BASE}${API_PREFIX}/tokens/"
# Cluster-local path (not under /api/content-sources/); same host works in local make run.
VALIDATE_URL="${CS_BASE}/internal/lightwell/tokens/validate"
REPOS_URL="${CS_BASE}${API_PREFIX}/repositories/?origin=lightwell"

echo "==> Phase A: CS identity + Bearer tokens"
echo "    CS_BASE=${CS_BASE}"
echo "    REPO_UUID=${REPO_UUID}"

code=$(http_code GET "${REPOS_URL}" -H "$(header_user)")
expect_status "identity list lightwell repos" "200" "${code}"

code=$(http_code GET "${PACKAGES_URL}" -H "$(header_user)")
expect_status "non-admin get packages" "403" "${code}"

code=$(http_code GET "${PACKAGES_URL}" -H "$(header_admin)")
expect_status "org-admin get packages" "200" "${code}"

# Platform identity middleware returns 400 for missing x-rh-identity (not 401).
code=$(http_code GET "${PACKAGES_URL}")
expect_status_one_of "no auth packages" "${code}" "400" "401"

CREATE_BODY='{"name":"smoke-token"}'
CREATE_RESP=$(curl -s -w '\n%{http_code}' -X POST "${TOKENS_URL}" \
  -H "$(header_admin)" \
  -H "Content-Type: application/json" \
  -d "${CREATE_BODY}")
CREATE_CODE=$(echo "${CREATE_RESP}" | tail -n1)
CREATE_JSON=$(echo "${CREATE_RESP}" | sed '$d')
expect_status "org-admin create token" "201" "${CREATE_CODE}"

TOKEN=$(echo "${CREATE_JSON}" | jq -r '.token // empty')
TOKEN_UUID=$(echo "${CREATE_JSON}" | jq -r '.uuid // empty')
if [[ -z "${TOKEN}" || "${TOKEN}" == "null" ]]; then
  RESULTS+=("FAIL  create returned plaintext token")
  FAIL=$((FAIL + 1))
  TOKEN="missing"
fi

code=$(http_code GET "${PACKAGES_URL}" -H "Authorization: Bearer ${TOKEN}")
expect_status "bearer get packages" "200" "${code}"

code=$(http_code GET "${PACKAGES_URL}" -H "Authorization: Bearer not-a-real-token")
expect_status "bad bearer packages" "401" "${code}"

code=$(http_code POST "${TOKENS_URL}" \
  -H "$(header_user)" \
  -H "Content-Type: application/json" \
  -d "${CREATE_BODY}")
expect_status "non-admin create token" "403" "${code}"

code=$(http_code POST "${VALIDATE_URL}" \
  -H "Content-Type: application/json" \
  -H "X-Rh-Cs-Internal-Token: ${SERVICE_AUTH}" \
  -d "{\"token\":\"${TOKEN}\"}")
expect_status "internal validate good token" "200" "${code}"

code=$(http_code POST "${VALIDATE_URL}" \
  -H "Content-Type: application/json" \
  -H "X-Rh-Cs-Internal-Token: ${SERVICE_AUTH}" \
  -d '{"token":"lw_bad"}')
expect_status_one_of "internal validate bad token" "${code}" "401" "403"

if [[ -n "${TOKEN_UUID}" && "${TOKEN_UUID}" != "null" ]]; then
  code=$(http_code DELETE "${TOKENS_URL}${TOKEN_UUID}" -H "$(header_admin)")
  expect_status "org-admin revoke token" "204" "${code}"

  code=$(http_code GET "${PACKAGES_URL}" -H "Authorization: Bearer ${TOKEN}")
  expect_status "revoked bearer packages" "401" "${code}"

  code=$(http_code POST "${VALIDATE_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Rh-Cs-Internal-Token: ${SERVICE_AUTH}" \
    -d "{\"token\":\"${TOKEN}\"}")
  expect_status_one_of "internal validate revoked token" "${code}" "401" "403"
fi

if [[ -n "${CONTENT_URL}" ]]; then
  echo ""
  echo "==> Pulp content checks (EXPECT_PULP_GUARDED=${EXPECT_PULP_GUARDED})"
  # Create a fresh token for content checks if we revoked the previous one
  CREATE_RESP=$(curl -s -w '\n%{http_code}' -X POST "${TOKENS_URL}" \
    -H "$(header_admin)" \
    -H "Content-Type: application/json" \
    -d '{"name":"smoke-content-token"}')
  CREATE_CODE=$(echo "${CREATE_RESP}" | tail -n1)
  CREATE_JSON=$(echo "${CREATE_RESP}" | sed '$d')
  CONTENT_TOKEN=$(echo "${CREATE_JSON}" | jq -r '.token // empty')
  CONTENT_TOKEN_UUID=$(echo "${CREATE_JSON}" | jq -r '.uuid // empty')
  expect_status "create content smoke token" "201" "${CREATE_CODE}"

  if [[ "${EXPECT_PULP_GUARDED}" == "1" ]]; then
    code=$(http_code GET "${CONTENT_URL}")
    expect_status_one_of "content no auth (guarded)" "${code}" "401" "403"

    code=$(http_code GET "${CONTENT_URL}" -H "Authorization: Bearer ${CONTENT_TOKEN}")
    expect_status "content with bearer (guarded)" "200" "${code}"

    if [[ -n "${CONTENT_TOKEN_UUID}" && "${CONTENT_TOKEN_UUID}" != "null" ]]; then
      http_code DELETE "${TOKENS_URL}${CONTENT_TOKEN_UUID}" -H "$(header_admin)" >/dev/null
      code=$(http_code GET "${CONTENT_URL}" -H "Authorization: Bearer ${CONTENT_TOKEN}")
      expect_status_one_of "content revoked bearer (guarded)" "${code}" "401" "403"
    fi
  else
    code=$(http_code GET "${CONTENT_URL}")
    expect_status "content no auth (unguarded expected)" "200" "${code}"
    RESULTS+=("NOTE  Pulp content is still open (EXPECT_PULP_GUARDED=0). This is expected until the Pulp token guard ships.")
  fi
fi

echo ""
echo "==> Results"
for line in "${RESULTS[@]}"; do
  echo "  ${line}"
done
echo "  ${PASS} passed, ${FAIL} failed"

if [[ "${FAIL}" -ne 0 ]]; then
  exit 1
fi
exit 0
