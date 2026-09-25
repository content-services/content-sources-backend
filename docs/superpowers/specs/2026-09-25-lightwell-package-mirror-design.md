# Lightwell Package Mirror — Design

Date: 2026-09-25
Status: Approved (design); pending implementation plan

## Problem

`GET /lightwell/packages` and `GET /lightwell/package_versions` are slow when
results span multiple repositories. Today the handler
(`pkg/handler/lightwell_packages.go`) fetches live from Pulp/Tang: for the
multi-repo case it fans out one goroutine per Lightwell repo, pulls the
*entire* package catalog of every repo into memory (`aggregatePackages` /
`aggregatePackageVersions` → `fetchAllPages`), then sorts, counts, and
paginates in-process. Pagination, totals, and filtering cannot be pushed down
to the source when data crosses repositories, so the cost scales with the
total catalog size rather than the requested page.

A single-repo "fast path" exists for `ListPackages` only (it pushes
offset/limit to Pulp/Tang); `package_versions` has no fast path.

## Goal

Mirror Lightwell package and package-version data into Postgres so the two
cross-repo list endpoints become plain SQL queries with filtering, sorting,
pagination, and counts pushed into the database. Keep the mirror fresh with a
periodic import that only does work when a Lightwell repo's version changes.

## Scope

In scope:

- Two mirror tables plus one column on `repository_configurations`.
- A 15-minute import command (OpenShift CronJob → `external-repos` subcommand,
  run inline).
- Rewriting **only** the two cross-repo list endpoints
  (`/lightwell/packages`, `/lightwell/package_versions`) — both the main-API
  and Lightwell v0.1 registrations — to read from the mirror.

Out of scope:

- The per-repo detail endpoints (`repositories/:uuid/packages`, maven/python
  package detail + versions). They query a single repo live and are already
  fast; they keep their current behavior.
- Any change to the advisory/vulnerability mirrors (reused as-is for CVE
  filtering).

## Decisions

These were settled during brainstorming and are fixed for the implementation:

1. **Sync trigger:** a dedicated 15-minute cron, *not* piggybacking on the
   existing snapshot pipeline. Deliberately avoid "snapshot" terminology to
   prevent confusion with the existing snapshotting feature — this feature is
   called **import**.
2. **Version tracking column:** `last_import_repository_version` on
   `repository_configurations`.
3. **Table shape:** two generic tables (`lightwell_packages`,
   `lightwell_package_versions`), not per-ecosystem tables.
4. **Ecosystem is not stored on the package.** A Lightwell repo has a single
   content type, so ecosystem is derived from `repositories.content_type` at
   read time (reachable via `repository_configuration_uuid`).
5. **`purl` is persisted** on the version row (computed once at import).
   `coordinates` remains computed at read time.
6. **Cutover:** DB-only. An un-imported repo simply contributes no packages
   until the cron populates it (first import within 15 min of deploy). The old
   live aggregation path is deleted.
7. **Import write strategy:** upsert changed/new rows and delete only the diff
   (removed rows), rather than full delete-and-reinsert. Deletions are the
   uncommon case; this keeps UUIDs stable and only moves `updated_at` on real
   changes.
8. **Import runs inline** in the CLI command with bounded concurrency,
   following `sync-lightwell-advisories` — not the worker task queue.

## Architecture

Three pieces:

1. **Mirror tables** in the shared Postgres DB, read via the existing
   sqlc/pgx store under `pkg/lightwell/db/` (the same access layer used by the
   advisory and vulnerability mirrors).
2. **Import command** — a new `external-repos` subcommand run by an OpenShift
   CronJob every 15 minutes. Runs inline; for each Lightwell repo it resolves
   the current Pulp repository-version href, compares it to
   `last_import_repository_version`, and only when changed pulls the full
   catalog and reconciles the mirror rows for that repo.
3. **Rewritten read path** — the two cross-repo list endpoints become sqlc
   queries with all filtering/sorting/pagination/counts in SQL.

## Data model

### `repository_configurations` (existing table)

Add `last_import_repository_version text` (nullable). Stores the Pulp
repository-version href last imported for that repo. `NULL` = never imported.
Non-destructive add-column migration.

### `lightwell_packages` (new)

One row per (repo config, group, name):

- `uuid` PK, `created_at`, `updated_at`
- `repository_configuration_uuid` → FK `repository_configurations(uuid)`
  **ON DELETE CASCADE**
- `name`
- `group` (nullable; Maven only)
- Unique index on `(repository_configuration_uuid, group, name)`

Ecosystem is *not* a column — derived from `repositories.content_type` via the
repo config join.

### `lightwell_package_versions` (new)

One row per version:

- `uuid` PK, `created_at`, `updated_at`
- `lightwell_package_uuid` → FK `lightwell_packages(uuid)` **ON DELETE CASCADE**
- `repository_configuration_uuid` (denormalized) → FK
  `repository_configurations(uuid)` **ON DELETE CASCADE** — lets the
  package_versions endpoint filter/join without going through the packages table
- `version`
- `release` (nullable)
- `published_at` (nullable; the upstream version timestamp, `CreatedAt` in the
  current API)
- `purl` (computed at import, persisted)
- Unique index on `(lightwell_package_uuid, version)`
- Index on `(repository_configuration_uuid)`

`coordinates` is computed at read time in the response mapper (deterministic
formatting of group/name/version per ecosystem; carries no extra information).

## Import command

New subcommand `import-lightwell-packages` →
`commands.ImportLightwellPackagesAction`, with a `--force` flag (mirrors
`sync-lightwell-advisories`). Registered as an OpenShift `CronJob` in
`deployments/deployment.yaml` on `*/15 * * * *`. Implementation in
`pkg/external_repos/commands/import_lightwell_packages.go`, laid out like
`sync_lightwell_advisories.go`, running inline with bounded concurrency (reuse
the `maxConcurrentFetches = 10` pattern).

Per invocation:

1. Load all Lightwell repos (internal fetch for `LightwellOrg` +
   `LightwellDemoOrg`), filtered to content types maven/python/npm.
   Entitlement is **not** applied here — the mirror holds everything;
   entitlement is enforced at read time.
2. Fan out over repos (bounded concurrency). For each repo:
   - Resolve domain + Pulp client and the repo href
     (`Domain.FetchOrCreateDomain` → `PulpClient.WithDomain` →
     `ResolveRepositoryFromBasePath`), then read the repo's current
     latest-version href.
   - Compare to `last_import_repository_version`. If equal and not `--force`,
     **skip — no work** (the common case).
   - If changed, pull the full catalog: Maven via `PulpClient.ListMavenPackages`
     (paged); Python/npm via the Tang client (paged). Build package rows +
     version rows, computing `purl` per version.
3. Persist per repo in a single **transaction**:
   - **Upsert packages** — `ON CONFLICT (repository_configuration_uuid, group,
     name) DO UPDATE` (bump `updated_at`), `RETURNING uuid` to attach versions
     to stable package UUIDs.
   - **Upsert versions** — `ON CONFLICT (lightwell_package_uuid, version) DO
     UPDATE` (refresh `purl`/`release`/`published_at`/`updated_at`).
   - **Delete the diff** — remove versions for this repo not in the newly
     imported set, then packages for this repo left with no versions.
   - Set `last_import_repository_version` to the new href.
4. Per-repo errors are logged and collected (`errors.Join`); processing
   continues with the next repo — same resilience as the advisory sync.

## Read path / API rewrite

### sqlc queries (`pkg/lightwell/db/queries/packages.sql`)

- `ListLightwellPackages` + count — joins `lightwell_packages` →
  `repository_configurations` (rc) → `repositories` (r, for
  `content_type`/name), `array_agg`s versions for the `Versions[]` /
  `LatestReleases` response fields, and pushes down name search, ecosystem
  (= `r.content_type`), repository, security_level, ordering, limit/offset.
- `ListLightwellPackageVersions` + count — joins versions → packages → rc → r;
  same filters; plus `ResolvesCveID` / `VulnerableToCveID` reproduced as SQL
  joins/subqueries against the already-mirrored `lightwell_advisories`,
  preserving today's fixed-version semantics from
  `filterVersionsByResolvingCve` / `filterVersionsByVulnerableCve`.

### Entitlement (CLAUDE.md requirement)

Both queries scope org to the Lightwell orgs and enforce the entitled-features
token match on `rc.feature_name`, exactly like `advisories.sql`. Because we now
query `lightwell_packages` directly (not through `RepositoryConfig.List`), this
guard is mandatory and lives in the query. The handler calls
`GetEntitledFeatures` and passes the tokens in.

### DAO + handler

- New `LightwellPackage` DAO wrapping the querier, added to
  `pkg/dao/interfaces.go` + the registry; regenerate mocks (`mockery`).
- `LightwellPackagesHandler.ListPackages` / `ListPackageVersions` keep request
  and filter parsing and response mapping (compute `coordinates`, read `purl`
  from the row), but call the new DAO instead of Pulp/Tang.
- Delete `aggregatePackages`, `aggregatePackageVersions`, `fetchAllPages`, the
  per-repo fetch helpers, and the single-repo fast path — for these two
  endpoints only. `TangClient` / `PulpClient` handler fields stay only if still
  used by the untouched per-repo detail endpoints.

Response types in `pkg/api/lightwell_packages.go` are unchanged.

## Migrations & build-pipeline sync

Migration `db/migrations/<timestamp>_add_lightwell_packages.up/.down.sql`
(wrapped in `BEGIN;/COMMIT;`):

- Add nullable `last_import_repository_version` to `repository_configurations`.
- Create `lightwell_packages` and `lightwell_package_versions` with FKs
  (ON DELETE CASCADE) and unique indexes above.
- Update `db/migrations.latest` to the new timestamp.

Per CLAUDE.md:

- Mirror the new tables/column into `pkg/lightwell/db/schema.sql`, add
  `pkg/lightwell/db/queries/packages.sql`, run `make sqlc-generate-lightwell`,
  and verify `store/*.sql.go`, `models.go`, `querier.go` regenerate cleanly.
- Regenerate mocks (`mockery`) for the new DAO interface methods.
- Run `make openapi-doc` if any `pkg/api` type changes (response shapes are
  unchanged, so likely no drift — verify `git diff --exit-code
  api/openapi.json`).
- `golangci-lint run --timeout=5m` (v2) before commit.

## Testing

- **Import command** — unit tests with mocked Pulp/Tang + DAO: unchanged
  version → no writes; changed version → upsert of new/changed
  packages+versions and deletion of removed ones; per-repo error isolation.
- **sqlc queries / DAO** — DB-backed tests (existing DAO test pattern) covering
  pagination/count, ecosystem/name/repository/security_level filters,
  entitlement filtering (entitled vs non-entitled `feature_name`), and both CVE
  filters against seeded `lightwell_advisories`.
- **Handler** — tests asserting the endpoints read from the mirror and no
  longer call Pulp/Tang, with entitlement enforced.
