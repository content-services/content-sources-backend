# Handoff: Lightwell access tokens UI (content-sources-frontend)

Audience: an agent implementing Console UI in `~/Work/content-sources-frontend` (or the content-sources-frontend repo).

Content Sources backend already owns token lifecycle and Bearer auth for the CS API. This handoff covers **org-admin token management UI only**.

Related:

- API types: [pkg/api/lightwell_tokens.go](../pkg/api/lightwell_tokens.go)
- Local smoke (CS side): [scripts/smoke_lightwell_tokens.sh](../scripts/smoke_lightwell_tokens.sh)
- Pulp validate contract (do not call from the browser): [lightwell_token_pulp_contract.md](lightwell_token_pulp_contract.md)
- Pulp guard handoff: [lightwell_tokens_pulp_handoff.md](lightwell_tokens_pulp_handoff.md)

## Demo contribution

Full E2E demo needs org-admins to create and revoke tokens in the Lightwell UI (plaintext shown once). CS Bearer package access already works without UI.

## Public CS API contract

Base path: `/api/content-sources/v1/` (also `/v1.0/`). Auth: Console `x-rh-identity` only — **Bearer tokens cannot manage tokens**.

| Method | Path | Behavior |
|--------|------|----------|
| `POST` | `/tokens/` | Create. Body: `name` (required), `access_level` (required: `validated` or `remediated`), optional `user_id` (defaults to caller), optional `expires_at` (default/max 365 days). Response **201** includes plaintext `token` **once**. |
| `GET` | `/tokens/` | List org tokens. Metadata only (`token_prefix`, never full secret). |
| `DELETE` | `/tokens/{uuid}` | Revoke. **204**. |

### Access levels

| `access_level` | May access repo `security_level` |
|----------------|----------------------------------|
| `validated` | `validated` and `remediated` |
| `remediated` | `remediated` only |

Scope applies to token Bearer use and Pulp validate only. Console org-admin browsing is unchanged.

### Response fields (list / create metadata)

`uuid`, `org_id`, `user_id`, `name`, `access_level`, `token_prefix`, `expires_at`, `revoked_at`, `last_used_at`, `created_at`. Create also returns `token`.

### Server gates (UI must mirror)

All of the following are enforced by the backend:

1. `features.lightwell` enabled and accessible for the caller (`GET /api/content-sources/v1/features/`).
2. Identity type `User` with `user.is_org_admin == true`.
3. RBAC: repositories `write` (create/revoke) / `read` (list).
4. Org has `lightwell-network` entitlement (Feature Service).

**Do not** call `POST /internal/lightwell/tokens/validate` from the browser. That path is cluster-local for Pulp only and is outside `/api/content-sources/`.

## Where to implement (frontend)

Work in the **Lightwell federated app**, not main Insights Content admin tabs.

| Area | Path |
|------|------|
| App shell / routes | `src/LightwellApp.tsx` |
| Pages | `src/Pages/Lightwell/` (add `Tokens/`) |
| Hand-written API clients | `src/services/**` (UI does **not** use OpenAPI codegen) |
| Features typing | `src/services/Features/FeatureApi.ts` |
| Nav helpers | `src/Hooks/Lightwell/navigation/` |
| Permissions empty state | `src/components/NoPermissionsPage/` |

Production UI pattern: hand-written axios in `*Api.ts` + TanStack Query in `*Queries.ts` (see `src/services/Content/`).

OpenAPI-generated `LightwellTokensApi` exists only under Playwright (`_playwright-tests/test-utils` submodule pointing at backend). Optional for e2e tests; not for production UI.

## Implementation map

1. **API layer**
   - Add `src/services/LightwellTokens/LightwellTokensApi.ts`
   - Add `LightwellTokensQueries.ts` (list query; create/revoke mutations; invalidate list; error toasts like other services)

2. **Feature typing**
   - Extend `Features` with `lightwell?: Feature` (backend already exposes it; FE type today often omits it)

3. **Org-admin helper**
   - Small hook (e.g. `Hooks/Lightwell/useIsOrgAdmin.ts`) via Chrome `auth.getUser()` → `identity.user.is_org_admin`
   - Org admin is **not** checked anywhere in the frontend today; backend still enforces it

4. **UI**
   - `Pages/Lightwell/Tokens/TokensTable.tsx` — columns: name, access level, `token_prefix`, user, expires, last used, created; revoke action
   - `CreateTokenModal.tsx` — name, required `access_level` (validated = full network; remediated = remediated only), optional expiry; on success show PatternFly `ClipboardCopy` (or `LabeledClipboardCopy`) of plaintext + clear “shown once” warning; do not persist full token in list state
   - `RevokeTokenModal.tsx` — confirm + DELETE (mirror existing delete modals)

5. **Routing / entry**
   - Add a `tokens` route in `LightwellApp.tsx` (sibling of repos)
   - Extend Lightwell nav helpers / destination keys
   - Entry: toolbar button on `RepositoriesTable` header and/or dedicated nav item
   - Show only when `features?.lightwell?.enabled && features.lightwell?.accessible && isOrgAdmin`
   - Non-admins: omit nav or render `NoPermissionsPage`

6. **Reuse**
   - Table/header: `Pages/Lightwell/Repositories/RepositoriesTable.tsx`
   - Modal shell: `ConnectRepositoryModal.tsx`
   - Clipboard: connect snippets / `LabeledClipboardCopy`
   - Feature-gated tabs pattern: main app `RepositoryLayout.tsx` (reference only)

7. **Follow-up (not blocking CRUD)**
   - Connect-repo flow today documents Red Hat service accounts (`connectSnippets.ts`). Product may later switch snippets to Lightwell access tokens — treat as a separate change.

## Acceptance criteria

- Org admin with Lightwell enabled: create → see plaintext once → list shows prefix only → revoke → list updates.
- Non-admin: no create/revoke UI (or NoPermissions); forced API call → **403**.
- Jest coverage for table/modals (see existing `RepositoriesTable.test.tsx` patterns).
- Optional Playwright using backend-generated `LightwellTokensApi` after backend merges.

## Out of scope

- Pulp content guard or any download enforcement.
- CS middleware, migrations, or validate API changes.
- Demo-org (`lightwell-network-demo`) PAT path unless product asks later.
- Putting token management under main Insights `Routes/index.tsx` unless product explicitly wants it there.

## Local verify (with backend)

1. Backend `make run` with `features.lightwell.enabled`, pepper/validate secret configured, Lightwell seeded.
2. As org-admin identity: create/list/revoke via UI against local CS.
3. Optional: `REPO_UUID=... ./scripts/smoke_lightwell_tokens.sh` for CS Bearer matrix (UI-independent).
