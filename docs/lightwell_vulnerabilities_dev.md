# Lightwell vulnerabilities — local development

Lightwell vulnerabilities live in the same backend as the rest of content-sources: database migrations, compose, and tests follow the normal app workflow. This doc covers the lightwell-specific seed data, sqlc store, schema reference, and read API.

## Local setup

Use the standard dev environment (see [README.md](../README.md)): copy `configs/config.yaml.example` to `configs/config.yaml`, then:

```bash
make compose-up
```

If you hit migration issues on an old local DB (e.g. from superseded lightwell migrations during development), reset and re-migrate:

```bash
make test-db-migrations
```

## Apply dev seed data

The seed script loads 52 mock vulnerabilities from `lightwell-vulnerabilities-2026-08-18.json` into two demo customers (`demo-customer-1`, `demo-customer-2`). Two rows set `duplicate_of` to a canonical `vulnerability_id` (not present in the mock):

```bash
psql "sslmode=disable dbname=content user=content host=localhost port=5433 password=content" -f db/seeds/lightwell_vulnerabilities.sql
```

Or through the compose postgres container:

```bash
docker compose exec -T postgres-content psql "sslmode=disable dbname=content user=content host=localhost port=5432 password=content" -f - < db/seeds/lightwell_vulnerabilities.sql
```

Adjust connection parameters to match your `configs/config.yaml` if they differ from the compose defaults.

Advisories for those vulnerabilities are a second script, applied locally after the vulnerability seed. It inserts one advisory (and a release row per `published_versions` entry) only for seed rows whose stage is `Lightwell Network`, using the vulnerability purl as `package_name` and `component_version` as `package_version`:

```bash
psql "sslmode=disable dbname=content user=content host=localhost port=5433 password=content" -f db/seeds/lightwell_advisories.sql
```

Or through the compose postgres container:

```bash
docker compose exec -T postgres-content psql "sslmode=disable dbname=content user=content host=localhost port=5432 password=content" -f - < db/seeds/lightwell_advisories.sql
```

## Read API

With the API running, authenticated clients can call:

- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/customers/` — distinct customer IDs
- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/?customer_id=demo-customer-1` — distinct Lightwell support ticket IDs for a customer
- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1` — filtered, paginated list with aggregates

`customer_id` is required on the list and `ltwlsupt-ticket-ids` endpoints. Filters (`severity`, `status`, `complexity`, `ltwlsupt_ticket_id`, `flag`) accept comma-separated values. `flag` accepts `embargo` and `duplicate` (OR). `search` requires at least 2 characters when provided.

## Beacon status mapping

Jira workflow status is stored as a Beacon status. A published fix promotes **Validation** to **Lightwell Network**. Once stored, Lightwell Network stays Lightwell Network if a later sync would only move it back to Validation.

| Jira status | Stored as |
| --- | --- |
| New | Submitted |
| Backlog | Classified |
| To Do | Classified |
| In Progress | Fix in Progress |
| Review | Fix in Progress |
| On Hold | Fix in Progress |
| Verified | Validation |
| Release Pending | Validation |
| Released | Validation |
| Closed | Validation |
| Anything else | Submitted |

Jira resolution can override that, or drop the issue. Won't Do and Not a Bug are stored as **Unremediated** with `resolution_reason` set to the explanation. The database keeps the Jira selection; the list API returns the explanation sentence. Those resolution names are not stored.

| Jira resolution | Result |
| --- | --- |
| Done | Stored from the workflow status above. No explanation. |
| Won't Do | Stored as Unremediated. Explanation comes from Lightwell Closure Reasoning Context. Selection **Not a Customer** is ignored and the issue is deleted. |
| Not a Bug | Stored as Unremediated. Explanation comes from VEX Justification. |
| Duplicate | Ignored. Deleted if it was stored before. |
| Obsolete | Ignored. Deleted if it was stored before. |
| Cannot Reproduce | Ignored. Deleted if it was stored before. |
| Won't Fix | Ignored. Deleted if it was stored before. |

## Run tests

Integration tests use the configured database and roll back per test:

```bash
CONFIG_PATH="$(pwd)/configs/" go test ./pkg/lightwell/db/store/... ./pkg/dao/... ./pkg/handler/...
```

Or via make (runs all `pkg/` tests):

```bash
make test-unit
```

## Regenerate sqlc store

After changing `pkg/lightwell/db/queries/*.sql` or the migration schema:

```bash
make sqlc-generate-lightwell
```

This runs the sqlc version pinned in `mk/sqlc.mk` (the same pin used by the `sqlcdiff` CI job). Generated code is written to `pkg/lightwell/db/store/`.

sqlc uses `pkg/lightwell/db/schema.sql` (final table snapshot plus `lightwell_filtered_vulnerabilities`) rather than parsing rename migrations directly. Update that file when the lightwell schema changes. List/count queries share filters through that function because sqlc has no query fragments.
