# Lightwell packages — local development

The package mirror is filled by `external-repos import-lightwell-packages`, which reads Maven from Pulp and Python and npm from Tang. This doc covers the manual local seed and how that job is meant to run.

## Apply dev seed data

The seed is not applied by `dbmigrate`. `scripts/create_lightwell_repo.sh` creates the Pulp repositories, imports them into the app, imports the Pulp catalog into the package mirror, and then loads vulnerabilities, advisories, and package rows onto those repositories:

```bash
./scripts/create_lightwell_repo.sh
```

The database seed uses the compose database on port 5433. Override it with `LIGHTWELL_SEED_PSQL` when your `configs/config.yaml` uses different settings.

Packages are written onto the repositories from `pkg/external_repos/lightwell_repos.json`, in org `-3`:

| Name | Packages |
| --- | --- |
| `lightwell/java/validated` | Upstream versions, empty release. No advisories. |
| `lightwell/java/remediated` | Advisory `fixed_versions`, including `rhlw`. |
| `lightwell/python/validated` | Upstream versions, empty release. No advisories. |
| `lightwell/python/remediated` | Advisory `fixed_versions`, including `rhlw`. |

Each version row has its own summary, license, project URL, and author. Python rows also set `author_email`. Maven `author_email` is empty. There is no npm seed repository. JavaScript and C# advisories stay on `lightwell/seed`.

The package seed runs after `import-lightwell-packages` and replaces the mirror rows for those four repositories, so the lists match the advisories. Pulp still has the catalog used by the endpoints that read Pulp directly.

`scripts/seed_lightwell_dev.sh` reloads only the database seed. It skips the vulnerability inserts when those rows already exist. The four repositories must already be present.

## Package import job

One cron runs `import-lightwell-packages` every 15 minutes with `concurrencyPolicy: Forbid`. It imports Maven, Python, and npm. If a run no longer finishes inside that interval, split it into three jobs using `--ecosystem maven`, `--ecosystem python`, and `--ecosystem npm`. Two jobs must never import the same repository.

`OPTIONS_LIGHTWELL_PACKAGE_IMPORT_FORCE_AT` forces one import without changing the cron command. Set a new RFC3339 value, for example `2026-10-09T13:00:00Z`. After a repository succeeds it stores that value, so the same value does not import it again. `--force` still imports the process you start immediately.

Stage and prod can import Maven through Pulp. Python and npm go through Tang, and Tang reads Pulp's Postgres (`clients.pulp.database`), not the Pulp HTTP API, so those ecosystems are not available everywhere yet. Keep the app `database.*` settings on the local database. Do not point the app database at a stage or prod database.

## CVE filters

`resolves_cve_id` matches an advisory when `la.package_name = p.name` and `pv.version` is one of `fixed_versions`. Maven advisories store `package_name` as `group:artifact`. The package row stores the artifact in `name` and the group in `package_group`, so that name comparison does not match Maven until the predicate changes. Do not store the coordinate in `name`.

`pv.version` is the full stored string, including a rebuild suffix, so it can match `fixed_versions` that contain `rhlw`. The public list returns `version` as the upstream part and `release` as the suffix.
