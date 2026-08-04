# Lightwell access tokens — Pulp content guard contract

Content Sources (this service) owns Lightwell access token lifecycle and an internal validate API. Pulp (or a `pulp_service` content guard) owns download-time enforcement by calling that API.

## Local Pulp note

Developer compose ([compose_files/pulp/docker-compose.yml](../compose_files/pulp/docker-compose.yml)) runs `quay.io/redhat-services-prod/pulp-services-tenant/pulp:latest`, which includes Red Hat `pulp_service` extensions (for example `ServiceFeatureContentGuard`). It is not bare upstream pulpcore.

Lightwell maven/python distributions are created **without** a `content_guard` today. Until a token guard is attached, content URLs remain publicly reachable.

## CS validate API (cluster-local only)

This endpoint is **not** part of the public Content Sources API. It is mounted at the server root (like `/ping`), **outside** `/api/content-sources/`, so it is not routed through console.redhat.com / 3scale. Pulp must call the in-cluster Content Sources service URL, never the public console hostname.

- **Method / path:** `POST /internal/lightwell/tokens/validate`
- **Example in-cluster URL:** `http://content-sources-backend-service:8000/internal/lightwell/tokens/validate` (service name may vary by env)
- **Not published:** absent from public OpenAPI; not reachable as `https://console.redhat.com/api/content-sources/...`
- **Auth:** header `X-Rh-Cs-Internal-Token` must match CS config `options.lightwell_validate_secret`
- **Body:**
  ```json
  { "token": "<raw bearer token>", "path": "/optional/request/path" }
  ```
- **Success (200):**
  ```json
  { "org_id": "...", "user_id": "...", "token_uuid": "..." }
  ```
- **Failure:** `401` (bad/missing token or secret), `403` (org lacks `lightwell-network` entitlement)

Entitlement is re-checked on **every** validate call.

## Required Pulp-side behavior

1. Add a content guard type usable on Lightwell domain maven/python distributions.
2. On each content request, read `Authorization: Bearer <token>` (optionally also Basic with the token as the password for Maven tooling).
3. Call the CS validate endpoint with the shared secret. Prefer no positive cache, or seconds-scale only, so revoke and entitlement loss apply quickly.
4. Deny the download if CS returns non-2xx.
5. Rollout: deploy CS validate → deploy guard → attach guard to Lightwell distributions.

## CS API Bearer auth (already in this service)

Clients may call Content Sources with `Authorization: Bearer <token>` (no `x-rh-identity`). Token management routes (`/tokens/`) reject Bearer auth and require Console identity with `is_org_admin`.

## See also

- Frontend UI handoff (org-admin token CRUD in Lightwell app): [lightwell_tokens_frontend_handoff.md](lightwell_tokens_frontend_handoff.md)
- Pulp guard + CS attach / rollout handoff: [lightwell_tokens_pulp_handoff.md](lightwell_tokens_pulp_handoff.md)
- Local smoke matrix: [scripts/smoke_lightwell_tokens.sh](../scripts/smoke_lightwell_tokens.sh)
