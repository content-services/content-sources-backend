#!/usr/bin/env bash
#
# Human-paced Lightwell access-token demo for terminal recordings (VHS).
#
# Story focus: org-admin create → unauthorized denied (incl. non-admin users) →
# Bearer allowed → revoke → denied.
# HTTP is used under the hood; the screen shows outcomes, not traffic dumps.
# See docs/demos/README.md and scripts/smoke_lightwell_tokens.sh.
#
# Prerequisites:
#   - make run (API on CS_BASE)
#   - features.lightwell.enabled: true
#   - lightwell-network entitlement available for the demo org
#   - seeded Lightwell repo (REPO_UUID) with packages
#
# Usage:
#   REPO_UUID=<uuid> ./scripts/demo_lightwell_tokens.sh
#
set -euo pipefail

CS_BASE="${CS_BASE:-http://localhost:8000}"
ORG_ID="${ORG_ID:-9999}"
USER_ID="${USER_ID:-1111}"
REPO_UUID="${REPO_UUID:-}"
API_PREFIX="${API_PREFIX:-/api/content-sources/v1.0}"
STEP_PAUSE="${STEP_PAUSE:-2}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [[ -z "${REPO_UUID}" ]]; then
  echo "ERROR: REPO_UUID is required (seeded Lightwell repository UUID)." >&2
  echo "  Example: REPO_UUID=<uuid> ./scripts/demo_lightwell_tokens.sh" >&2
  exit 1
fi

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
dim() { printf '\033[2m%s\033[0m\n' "$*"; }
action() { printf '  %s\n' "$*"; }
allowed() { printf '  \033[32m→ %s\033[0m\n' "$*"; }
denied() { printf '  \033[31m→ %s\033[0m\n' "$*"; }

step() {
  echo ""
  bold "════════════════════════════════════════════════════════"
  bold "  $*"
  bold "════════════════════════════════════════════════════════"
  sleep "${STEP_PAUSE}"
}

pause() {
  sleep "${STEP_PAUSE}"
}

header_admin() {
  "${SCRIPT_DIR}/header_org_admin.sh" "${ORG_ID}" "${USER_ID}"
}

header_user() {
  "${SCRIPT_DIR}/header.sh" "${ORG_ID}" "${USER_ID}"
}

mask_token() {
  local token="$1"
  printf '%s…%s' "${token:0:7}" "${token: -4}"
}

# Quiet HTTP helpers — status codes only reach the story lines above.
http_code() {
  local method="$1"
  local url="$2"
  shift 2
  curl -s -o /dev/null -w '%{http_code}' --connect-timeout 5 --max-time 30 -X "${method}" "$@" "${url}"
}

http_body() {
  local method="$1"
  local url="$2"
  shift 2
  curl -s --connect-timeout 5 --max-time 30 -X "${method}" "$@" "${url}"
}

PACKAGES_URL="${CS_BASE}${API_PREFIX}/repositories/${REPO_UUID}/packages"
TOKENS_URL="${CS_BASE}${API_PREFIX}/tokens/"

cd "${ROOT_DIR}"

bold "Lightwell access tokens"
echo ""
action "Build systems need packages from Lightwell repositories."
action "Those repos should not be open to the world — and teams"
action "should not share a human Console login to fetch them."
echo ""
action "Only an org admin (in Console) or a valid access token"
action "can read packages. Regular user accounts are denied."
action "Org admins create tokens for CI; revoke anytime to cut"
action "access off without redeploying or rotating passwords."
echo ""
dim "This walkthrough: create → deny → allow → revoke."
pause
pause

# ---------------------------------------------------------------------------
step "1. Org admin creates an access token"
# ---------------------------------------------------------------------------

action "Creating token \"demo-ci-reader\"…"
CREATE_JSON=$(http_body POST "${TOKENS_URL}" \
  -H "$(header_admin)" \
  -H "Content-Type: application/json" \
  -d '{"name":"demo-ci-reader"}')
TOKEN=$(echo "${CREATE_JSON}" | jq -r '.token // empty')
TOKEN_UUID=$(echo "${CREATE_JSON}" | jq -r '.uuid // empty')
TOKEN_PREFIX=$(echo "${CREATE_JSON}" | jq -r '.token_prefix // empty')

if [[ -z "${TOKEN}" || "${TOKEN}" == "null" ]]; then
  echo "ERROR: create did not return a plaintext token (is Lightwell enabled? org admin?)" >&2
  exit 1
fi

allowed "created  (prefix ${TOKEN_PREFIX})"
action "Secret (shown once): $(mask_token "${TOKEN}")"
dim "  Copy it now — it will not be shown again."
pause

# ---------------------------------------------------------------------------
step "2. Access is denied for non-admin users and credential-less requests"
# ---------------------------------------------------------------------------

action "Reading packages with no credential…"
code=$(http_code GET "${PACKAGES_URL}")
denied "denied   (HTTP ${code})"
pause

action "Reading packages as a regular (non-admin) user…"
code=$(http_code GET "${PACKAGES_URL}" -H "$(header_user)")
denied "denied   (HTTP ${code})"
pause

action "Reading packages with a forged token…"
code=$(http_code GET "${PACKAGES_URL}" -H "Authorization: Bearer lw_not-a-real-token")
denied "denied   (HTTP ${code})"
pause

# ---------------------------------------------------------------------------
step "3. Access is allowed for org admins or valid tokens"
# ---------------------------------------------------------------------------

action "Reading packages with the access token…"
code=$(http_code GET "${PACKAGES_URL}" -H "Authorization: Bearer ${TOKEN}")
if [[ "${code}" == "200" ]]; then
  TOTAL=$(http_body GET "${PACKAGES_URL}?limit=1" -H "Authorization: Bearer ${TOKEN}" | jq -r '.total // 0')
  allowed "allowed  (HTTP ${code}, ${TOTAL} packages)"
else
  denied "unexpected (HTTP ${code})"
fi
pause

action "Reading packages as an org admin (Console)…"
code=$(http_code GET "${PACKAGES_URL}" -H "$(header_admin)")
if [[ "${code}" == "200" ]]; then
  allowed "allowed  (HTTP ${code})"
else
  denied "unexpected (HTTP ${code})"
fi
pause

# ---------------------------------------------------------------------------
step "4. Revoke closes the door"
# ---------------------------------------------------------------------------

action "Revoking the token…"
code=$(http_code DELETE "${TOKENS_URL}${TOKEN_UUID}" -H "$(header_admin)")
allowed "revoked  (HTTP ${code})"
pause

action "Reading packages with the same token…"
code=$(http_code GET "${PACKAGES_URL}" -H "Authorization: Bearer ${TOKEN}")
denied "denied   (HTTP ${code})"
pause

echo ""
bold "Done."
dim "Org admins mint tokens and can browse in Console; CI uses Bearer; revoke cuts token access immediately."
echo "DEMO_COMPLETE"
echo ""
