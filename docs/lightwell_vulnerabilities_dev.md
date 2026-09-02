# Lightwell vulnerabilities — local development

Lightwell vulnerabilities live in the same backend as the rest of content-sources: database migrations, compose, and tests follow the normal app workflow. This doc covers the lightwell-specific seed data, sqlc store, schema reference, and read API.

## Local setup

Use the standard dev environment (see [README.md](../README.md)): copy `configs/config.yaml.example` to `configs/config.yaml`, then:

```bash
make compose-up
```

That starts dependencies and runs all migrations, including `20260814120000_create_lightwell_vulnerabilities`, `20260818120000_add_lightwell_vulnerability_duplicate_of`, `20260818180000_add_lightwell_vulnerability_tickets`, `20260818190000_add_lightwell_filtered_vulnerabilities`, and `20260819131200_rename_lightwell_vulnerability_support_tickets` (per-customer Lightwell support ticket mapping used by API field `ltwlsupt_ticket_ids`).

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

## Read API

With the API running, authenticated clients can call:

- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/customers/` — distinct customer IDs
- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/?customer_id=demo-customer-1` — distinct Lightwell support ticket IDs for a customer
- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1` — filtered, paginated list with aggregates

`customer_id` is required on the list and `ltwlsupt-ticket-ids` endpoints. Filters (`severity`, `stage`, `complexity`, `ltwlsupt_ticket_id`, `flag`) accept comma-separated values. `flag` accepts `embargo` and `duplicate` (OR). `search` requires at least 2 characters when provided.

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
