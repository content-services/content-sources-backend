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

The seed script loads mock vulnerabilities into two demo customers (`demo-customer-1`, `demo-customer-2`). Statuses include On Hold and Unremediated. Each Unremediated row stores one supported `resolution_reason`, and the list API returns the customer explanation for that selection. Two rows set `duplicate_of` to a canonical `vulnerability_id` (not present in the mock):

```bash
psql "sslmode=disable dbname=content user=content host=localhost port=5433 password=content" -f db/seeds/lightwell_vulnerabilities.sql
```

Or through the compose postgres container:

```bash
docker compose exec -T postgres-content psql "sslmode=disable dbname=content user=content host=localhost port=5432 password=content" -f - < db/seeds/lightwell_vulnerabilities.sql
```

Adjust connection parameters to match your `configs/config.yaml` if they differ from the compose defaults.

Advisories and the package mirror are loaded by `scripts/create_lightwell_repo.sh` after it creates the Pulp repositories and imports them. That script inserts one advisory (and a release row per `published_versions` entry) only for seed rows whose stage is `Lightwell Network`, using the vulnerability purl as `package_name` and `component_version` as `package_version`. Java advisories are attached to `lightwell/java/remediated` or `lightwell/java/predisclosure`, Python advisories to `lightwell/python/remediated` or `lightwell/python/predisclosure`, and JavaScript advisories to `lightwell/javascript/remediated`. See [Lightwell packages](lightwell_packages_dev.md):

```bash
./scripts/create_lightwell_repo.sh
```

## Read API

With the API running, authenticated clients can call:

- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/customers/` — distinct customer IDs
- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/?customer_id=demo-customer-1` — distinct Lightwell support ticket IDs for a customer
- `GET /api/content-sources/v1/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1` — filtered, paginated list with aggregates

`customer_id` is required on the list and `ltwlsupt-ticket-ids` endpoints. Filters (`severity`, `status`, `complexity`, `ltwlsupt_ticket_id`, `flag`) accept comma-separated values. `flag` accepts `embargo` and `duplicate` (OR). `search` requires at least 2 characters when provided.

`sort_by` takes `field:direction`: field is `last_updated`, `submitted_date`, `cvss`, `severity`, `status`, or `vulnerability_id`; direction is `asc` or `desc` (default `last_updated:desc`). `severity` sorts by rank (Critical > Important > Moderate > Low). `status` sorts in pipeline order (Submitted, Classified, Fix in Progress, Validation, Lightwell Network), then On Hold, then Unremediated. Rows without a CVSS score, or with an unknown severity or status, sort last in both directions. `vulnerability_id` compares the prefix (e.g. `CVE-2026`), then the trailing number numerically, so `CVE-2026-9999` comes before `CVE-2026-10000`. Ties are broken by `vulnerability_id` in ascending text order, whatever the direction.

## Beacon status mapping

Jira workflow status is stored as a Beacon status. A published fix promotes **Validation** to **Lightwell Network**. Once stored, Lightwell Network stays Lightwell Network if a later sync would only move it back to Validation.

| Jira status | Stored as |
| --- | --- |
| New | Submitted |
| Backlog | Classified |
| To Do | Classified |
| In Progress | Fix in Progress |
| Review | Fix in Progress |
| On Hold | On Hold |
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

Ignored resolutions are logged at warning level with the message `beacon_ignored_resolution`, plus `issue`, `resolution`, and `detail` when the selection is Not a Customer.

## On Hold reasons

When a vulnerability's stored stage is **On Hold**, the sync also records why. The reasons come from the tickets linked to the vulnerability (the IVT), never from labels on the IVT itself. Only **open** blockers count, meaning the linked ticket's Jira status category is not `done`.

| Linked ticket | Link | Where the reason comes from |
| --- | --- | --- |
| Build Task | "is blocked by" | Its `blocked:<reason>` labels |
| Component Upgrade | "Incorporates" (either direction) | Its `blocked:<reason>` labels, plus `blocked-by-ticket` if an open ticket blocks the Component Upgrade itself (its own "is blocked by" link) |
| Any other issue type | "is blocked by" | No label needed: `blocked-by-ticket` |

A vulnerability's stored stage is On Hold when its own Jira status is On Hold, or when **any open Component Upgrade that incorporates it is On Hold** (Jira embeds that Component Upgrade's status name in the link). The second case applies only to vulnerabilities in Submitted, Classified, Fix in Progress or Validation that Jira does not show as Released or Closed/Done. Shipped vulnerabilities (Lightwell Network) and ones closed without a fix (Unremediated) are never changed. Such a vulnerability takes its reasons only from its On Hold Component Upgrades, and its own blocking tickets are not read. The stage is worked out on every run, so it goes back to its own stage, with no reasons, when the Component Upgrade leaves On Hold.

Each `blocked:<reason>` label has its prefix stripped and is lowercased to a slug. Slugs are de-duplicated, sorted, and stored in `lightwell_vulnerabilities.on_hold_reasons` (`TEXT[]`, never `NULL`). An open Build Task or Component Upgrade without a `blocked:` label contributes nothing, so an On Hold vulnerability can legitimately have no reasons. Any other stage saves an empty list, which clears reasons left from an earlier On Hold period.

The slugs the API knows how to word are `ant-build`, `native-build`, `missing-sources`, `invalid-license`, `validation-failure`, `version-nonexistent`, `root-gav-unidentified`, `build-system-unknown`, `unsupported-ecosystem`, `monorepo`, `synthesis-conflict`, `cumulative-build-failure`, `integration-failure`, `cant-build-policy`, and `blocked-by-ticket`.

The database keeps the slugs. The list API returns `on_hold_reasons` as customer-facing text, in stored order, and omits the field when there is nothing to show. The wording lives in one place, `onHoldReasonDisplay` in `pkg/dao/lightwell_vulnerability.go`. A slug that map does not contain is shown as "Blocked on a dependency", so a new Jira label never silently disappears.

The blockers' labels, issue links and issue type are batch-fetched once per sync for the open blockers of On Hold vulnerabilities and for the On Hold Component Upgrades of other vulnerabilities. If a blocker cannot be fetched, that vulnerability is counted as failed and not saved, so the reasons it already has are not wiped.

Observability:

- An On Hold vulnerability saved with no reasons is logged at **debug** level with the message `beacon_on_hold_without_reason`, plus `issue` and `open_blockers` (open Build Task and Component Upgrade blockers that had no `blocked:` label).
- A vulnerability saved as On Hold only because a Component Upgrade is On Hold is logged at **debug** level with the message `beacon_on_hold_from_component_upgrade`, plus `issue` and `on_hold_component_upgrades` (the keys).
- The job's final "Finished syncing" line includes `on_hold_without_reason` and `on_hold_from_component_upgrade`, the number of vulnerabilities in each case.
- Set `logging.level: debug` in `configs/config.yaml` to see the per-issue lines (the example config already does).

The demo seed gives the three On Hold rows one case each: `PROJ-3501` has `cumulative-build-failure` and `missing-sources`, `PROJ-3502` has `blocked-by-ticket`, and `PROJ-3503` has none. The seeder only loads into a database without the demo rows, so on a database seeded before this change, delete the demo rows (those with a `uuid` starting `00000000-0000-4000-8000-`) and run migrations again.

To run the real sync against Jira locally, with the database up and migrated, supply the Jira user and token from your team's password manager. Never commit them:

```bash
CLIENTS_JIRA_URL="https://redhat.atlassian.net/" CLIENTS_JIRA_USER="<username>" CLIENTS_JIRA_TOKEN="<token>" go run cmd/jobs/main.go sync-lightwell-vulnerabilities
```

This writes real vulnerabilities into your local database. Compare a few On Hold rows' `on_hold_reasons` against their Jira tickets.

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
