# Lightwell vulnerabilities — local development

Lightwell vulnerabilities live in the same backend as the rest of content-sources: database migrations, compose, and tests follow the normal app workflow. This doc covers the lightwell-specific seed data, sqlc store, and schema reference. HTTP handlers are not implemented yet.

## Local setup

Use the standard dev environment (see [README.md](../README.md)): copy `configs/config.yaml.example` to `configs/config.yaml`, then:

```bash
make compose-up
```

That starts dependencies and runs all migrations, including `20260814120000_create_lightwell_vulnerabilities`.

If you hit migration issues on an old local DB (e.g. from superseded lightwell migrations during development), reset and re-migrate:

```bash
make test-db-migrations
```

## Apply dev seed data

The seed script loads 52 mock vulnerabilities from `lightwell-vulnerabilities-2026-08-14.json` into two demo customers (`demo-customer-1`, `demo-customer-2`):

```bash
psql "sslmode=disable dbname=content user=content host=localhost port=5433 password=content" -f db/seeds/lightwell_vulnerabilities.sql
```

Or through the compose postgres container:

```bash
docker compose exec -T postgres-content psql "sslmode=disable dbname=content user=content host=localhost port=5432 password=content" -f - < db/seeds/lightwell_vulnerabilities.sql
```

Adjust connection parameters to match your `configs/config.yaml` if they differ from the compose defaults.

## Run tests

Integration tests use the configured database and roll back per test:

```bash
CONFIG_PATH="$(pwd)/configs/" go test ./pkg/lightwell/db/store/...
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

Generated code is written to `pkg/lightwell/db/store/`.

sqlc uses `pkg/lightwell/db/schema.sql` (final table snapshot) rather than parsing rename migrations directly. Update that file when the lightwell schema changes.
