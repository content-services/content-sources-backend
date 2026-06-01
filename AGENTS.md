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

## Commit guidelines

- PR titles should reference the tracking ticket: `<JIRA Number>: description`
- See [CONTRIBUTING.md](CONTRIBUTING.md) for full details.
