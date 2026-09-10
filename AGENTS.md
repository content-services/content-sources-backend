# AGENTS.md

Project: Backend for Red Hat's content sources service (Go + PostgreSQL)

Frontend: https://github.com/content-services/content-sources-frontend

## Overview

See [docs/architecture.md](docs/architecture.md) for a description of the service and its architecture.

## Dev environment setup

- Requires podman/docker with compose
- Copy `configs/config.yaml.example` to `configs/config.yaml`
- Run `make help` to see available targets
- See [README.md](README.md) for full setup instructions

## Where to look in the tree

| Area | Typical paths |
|------|---------------|
| REST API handlers | `pkg/handler/` |
| API route definitions | `pkg/router/` |
| Data access (DAO) layer | `pkg/dao/` |
| Domain models | `pkg/models/` |
| Background tasks/workers | `pkg/tasks/` |
| Kafka integration | `pkg/kafka/` |
| RBAC / middleware | `pkg/rbac/`, `pkg/middleware/` |
| Configuration | `pkg/config/` |
| Database migrations | `db/migrations/` |
| OpenAPI spec | `api/` |
| CLI entrypoints | `cmd/` |
| Compose / containers | `compose_files/`, `build/` |
| Deployment (ClowdApp) | `deployments/` |
| Integration / API tests | `_playwright-tests/`, `test/` |

## Code discipline

- Match existing Go style, package layout, and patterns in the touched area.
- Keep changes minimal and scoped to the task; avoid drive-by refactors.
- PRs should come with good tests.
- SQL migrations must be non-destructive (see [CONTRIBUTING.md](CONTRIBUTING.md) for the two-stage migration approach).

## Feature entitlement

Lightwell endpoints must verify the caller's org has access to Lightwell features. The `RepositoryConfig.List` DAO method calls `GetEntitledFeatures` internally and filters repos by `feature_name`, so handlers that query through it (packages, package_versions) get entitlement checking implicitly. Handlers that query other tables directly (advisories, vulnerabilities) must add their own guard — either by checking `GetEntitledFeatures` or by confirming the org has visible Lightwell repos via `RepositoryConfig.List`. When adding a new Lightwell endpoint, verify it is guarded.

## Lint before push

After making Go code changes, run the project linter before committing:

```bash
golangci-lint run --timeout=5m
```

The project uses golangci-lint **v2** (`.golangci.yaml` has `version: "2"`). Install with:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

If lint fails, use `--fix` to auto-correct formatting issues (e.g., `gci` import ordering), then re-run until `0 issues`.

## Build pipeline sync

After modifying SQL queries in `pkg/lightwell/db/queries/`:
- Run `make sqlc-generate-lightwell` to regenerate Go code
- Verify `pkg/lightwell/db/store/*.sql.go` and `models.go` match

After modifying handler swag annotations or API types in `pkg/api/`:
- Run `make openapi-doc` to regenerate `api/openapi.json` and `api/docs.go`
- CI checks `git diff --exit-code api/openapi.json` and fails on drift

After adding or renaming migration files in `db/migrations/`:
- Update `db/migrations.latest` to match the latest migration timestamp
- Keep `pkg/lightwell/db/schema.sql` in sync with the cumulative DDL

After adding methods to a `Querier` or DAO interface:
- Regenerate mocks (`mockery`) so mock types satisfy the full interface

## Generated files

Generated files (`api/docs.go`, `api/openapi.json`, sqlc output in `pkg/lightwell/db/store/`) on feature branches should match `origin/main` until post-merge regeneration. Do not commit regenerated output that only differs due to a rebase or local toolchain version.

## Commit guidelines

- PR titles should reference the tracking ticket: `<JIRA Number>: description`
- See [CONTRIBUTING.md](CONTRIBUTING.md) for full details.
