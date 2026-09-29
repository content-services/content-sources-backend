# Lightwell Package Mirror Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mirror Lightwell package/package-version data into Postgres and serve the two cross-repo list endpoints (`/lightwell/packages`, `/lightwell/package_versions`) from the mirror instead of live Pulp/Tang aggregation, kept fresh by a 15-minute import cron.

**Architecture:** Two new tables (`lightwell_packages`, `lightwell_package_versions`) plus a `last_import_repository_version` column on `repository_configurations`. A new `external-repos import-lightwell-packages` CLI subcommand (run by an OpenShift CronJob every 15 min) resolves each Lightwell repo's current Pulp latest-version href, and when it differs from the stored value, pulls the full catalog (Pulp for Maven, Tang for Python/npm) and reconciles the mirror rows via upsert-and-delete-diff. The endpoints become sqlc queries with filtering/sorting/pagination/counts pushed into SQL and entitlement enforced in the query.

**Tech Stack:** Go, PostgreSQL, GORM (writes), sqlc + pgx/v5 (reads), golang-migrate, urfave/cli/v2, zest Pulp client, tang.

**Spec:** `docs/superpowers/specs/2026-09-25-lightwell-package-mirror-design.md`

## Global Constraints

- Migrations are non-destructive and wrapped in `BEGIN;`/`COMMIT;`. Use `ADD COLUMN IF NOT EXISTS` / `DROP COLUMN IF EXISTS`, `DROP INDEX IF EXISTS`, `DROP TABLE IF EXISTS`; plain `CREATE TABLE` (no `IF NOT EXISTS`); index naming `idx_<tableabbrev>_<cols>`; FKs inline `REFERENCES ... ON DELETE CASCADE`.
- After adding/renaming migrations, update `db/migrations.latest` to the newest migration timestamp.
- Keep `pkg/lightwell/db/schema.sql` in sync with the cumulative DDL; after editing `pkg/lightwell/db/queries/*.sql` or `schema.sql`, run `make sqlc-generate-lightwell` and commit the regenerated `store/*.sql.go`, `models.go`, `querier.go`.
- After adding methods to a DAO interface or the `PulpClient` interface, run `make mock` to regenerate `pkg/dao/dao_mock.go` / `pkg/clients/pulp_client/pulp_client_mock.go`.
- Content type constants (exact values): `config.ContentTypeMaven = "maven"`, `config.ContentTypePython = "python"`, `config.ContentTypeNpm = "npm"`.
- The `group` value convention (must match existing handler behavior): Maven → `group_id`; Python → `""`; npm → `"@scope"` or `"-"` when unscoped (via `handler.ParseNpmPackageName`).
- Run `golangci-lint run --timeout=5m` (golangci-lint **v2**) before each commit; use `--fix` for gci/formatting.
- Response JSON shapes in `pkg/api/lightwell_packages.go` do NOT change, so `api/openapi.json` must not drift — verify `git diff --exit-code api/openapi.json` after handler edits; only run `make openapi-doc` if a swag annotation or `pkg/api` type actually changed.
- Generated files on this branch should match `origin/main` except for the intentional additions in this plan.

## Review Focus

- **Unresolvable repo during import** (distribution/base path resolves to nil or Pulp errors): the import must log and skip that one repo and continue the rest (per-repo error isolation via `errors.Join`), never aborting the whole run. → Task 6.
- **Version changed to an empty catalog** (all packages removed upstream): `SyncPackagesForRepository` with an empty input slice must delete all mirror rows for that repo. → Task 5.
- **Changed vs removed versions** (a version's `release`/`purl`/`published_at` changed; a version disappeared): upsert must update the changed row in place (stable uuid) and the delete-diff must remove only the disappeared rows. → Task 5.
- **Entitlement gate** (caller's `entitled_features` does not intersect a repo's `feature_name`, and the NULL/blank `feature_name` case): non-entitled repos' packages excluded; NULL/blank `feature_name` repos remain visible — matching `RepositoryConfig.List` semantics. → Task 4.
- **Filter semantics parity** (name substring is case-insensitive `ILIKE`; `security_level` match is case-insensitive equality; `repository` match is case-insensitive exact on repo config name): the SQL must reproduce the old in-memory `EqualFold`/substring behavior. → Task 4.

---

### Task 1: Migration, schema.sql, GORM models, migrations.latest

**Files:**
- Create: `db/migrations/<timestamp>_create_lightwell_packages.up.sql`
- Create: `db/migrations/<timestamp>_create_lightwell_packages.down.sql`
- Modify: `db/migrations.latest`
- Modify: `pkg/lightwell/db/schema.sql`
- Create: `pkg/models/lightwell_package.go`
- Modify: `pkg/models/repository_configuration.go:37` (add field after `FeatureName`)
- Test: `pkg/models/lightwell_package_test.go`

**Interfaces:**
- Produces: tables `lightwell_packages`, `lightwell_package_versions`; column `repository_configurations.last_import_repository_version`; GORM models `models.LightwellPackage`, `models.LightwellPackageVersion` (table names `lightwell_packages`, `lightwell_package_versions`); model field `RepositoryConfiguration.LastImportRepositoryVersion string` (column `last_import_repository_version`).

- [ ] **Step 1: Generate the migration files**

Run: `go run ./cmd/dbmigrate new create_lightwell_packages`

This creates the paired `.up.sql`/`.down.sql` under `db/migrations/` (seeded with `BEGIN;`/`COMMIT;`) and rewrites `db/migrations.latest` to the new timestamp automatically.

- [ ] **Step 2: Write the `.up.sql`**

Replace the generated `.up.sql` body with:

```sql
BEGIN;

ALTER TABLE repository_configurations
    ADD COLUMN IF NOT EXISTS last_import_repository_version TEXT;

CREATE TABLE lightwell_packages (
    uuid UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    package_group TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_packages_repo_group_name
    ON lightwell_packages (repository_configuration_uuid, package_group, name);

CREATE TABLE lightwell_package_versions (
    uuid UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lightwell_package_uuid UUID NOT NULL REFERENCES lightwell_packages(uuid) ON DELETE CASCADE,
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    version TEXT NOT NULL,
    release TEXT NOT NULL DEFAULT '',
    published_at TEXT NOT NULL DEFAULT '',
    purl TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_package_versions_pkg_version
    ON lightwell_package_versions (lightwell_package_uuid, version);

CREATE INDEX idx_lightwell_package_versions_repo
    ON lightwell_package_versions (repository_configuration_uuid);

COMMIT;
```

- [ ] **Step 3: Write the `.down.sql`**

```sql
BEGIN;

DROP TABLE IF EXISTS lightwell_package_versions;
DROP TABLE IF EXISTS lightwell_packages;

ALTER TABLE repository_configurations
    DROP COLUMN IF EXISTS last_import_repository_version;

COMMIT;
```

- [ ] **Step 4: Mirror the DDL into `pkg/lightwell/db/schema.sql`**

The `repository_configurations` stub in that file currently only has `uuid` and `feature_name`. sqlc needs the columns the new queries reference. Update the stub and append the two tables. Change the stub (around lines 5-8) to:

```sql
CREATE TABLE repository_configurations (
    uuid UUID PRIMARY KEY,
    org_id VARCHAR(255) DEFAULT NULL,
    name VARCHAR(255) DEFAULT NULL,
    repository_uuid UUID DEFAULT NULL,
    feature_name VARCHAR(255) DEFAULT NULL,
    last_import_repository_version TEXT DEFAULT NULL
);

CREATE TABLE repositories (
    uuid UUID PRIMARY KEY,
    content_type VARCHAR(255) NOT NULL DEFAULT 'rpm',
    security_level VARCHAR(255) DEFAULT NULL,
    origin VARCHAR(255) DEFAULT NULL
);
```

Then append at the end of the file:

```sql
CREATE TABLE lightwell_packages (
    uuid UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    package_group TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_packages_repo_group_name
    ON lightwell_packages (repository_configuration_uuid, package_group, name);

CREATE TABLE lightwell_package_versions (
    uuid UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lightwell_package_uuid UUID NOT NULL REFERENCES lightwell_packages(uuid) ON DELETE CASCADE,
    repository_configuration_uuid UUID NOT NULL REFERENCES repository_configurations(uuid) ON DELETE CASCADE,
    version TEXT NOT NULL,
    release TEXT NOT NULL DEFAULT '',
    published_at TEXT NOT NULL DEFAULT '',
    purl TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_lightwell_package_versions_pkg_version
    ON lightwell_package_versions (lightwell_package_uuid, version);

CREATE INDEX idx_lightwell_package_versions_repo
    ON lightwell_package_versions (repository_configuration_uuid);
```

> Note: `schema.sql` is not executed — it only feeds sqlc codegen. Adding the `repositories` table and extra `repository_configurations` columns here does not affect the real DB; it lets the Task 4 queries compile under sqlc.

- [ ] **Step 5: Write the GORM models**

Create `pkg/models/lightwell_package.go`:

```go
package models

const (
	TableNameLightwellPackage        = "lightwell_packages"
	TableNameLightwellPackageVersion = "lightwell_package_versions"
)

// LightwellPackage is one mirrored package (per repo config, group, name).
type LightwellPackage struct {
	Base
	RepositoryConfigurationUUID string `json:"repository_configuration_uuid" gorm:"not null"`
	Name                        string `json:"name" gorm:"not null"`
	Group                       string `json:"group" gorm:"column:package_group;not null;default:''"`
}

func (LightwellPackage) TableName() string { return TableNameLightwellPackage }

// LightwellPackageVersion is one mirrored version of a LightwellPackage.
type LightwellPackageVersion struct {
	Base
	LightwellPackageUUID        string `json:"lightwell_package_uuid" gorm:"not null"`
	RepositoryConfigurationUUID string `json:"repository_configuration_uuid" gorm:"not null"`
	Version                     string `json:"version" gorm:"not null"`
	Release                     string `json:"release" gorm:"not null;default:''"`
	PublishedAt                 string `json:"published_at" gorm:"not null;default:''"`
	Purl                        string `json:"purl" gorm:"not null;default:''"`
}

func (LightwellPackageVersion) TableName() string { return TableNameLightwellPackageVersion }
```

- [ ] **Step 6: Add the model field on RepositoryConfiguration**

In `pkg/models/repository_configuration.go`, add after the `FeatureName` line (`:37`):

```go
	LastImportRepositoryVersion string `json:"last_import_repository_version" gorm:"default:null"`
```

- [ ] **Step 7: Write a model smoke test**

Create `pkg/models/lightwell_package_test.go`:

```go
package models

import "testing"

func TestLightwellPackageTableNames(t *testing.T) {
	if LightwellPackage{}.TableName() != "lightwell_packages" {
		t.Fatalf("unexpected table name: %s", LightwellPackage{}.TableName())
	}
	if LightwellPackageVersion{}.TableName() != "lightwell_package_versions" {
		t.Fatalf("unexpected table name: %s", LightwellPackageVersion{}.TableName())
	}
}
```

- [ ] **Step 8: Run the test**

Run: `go test ./pkg/models/ -run TestLightwellPackageTableNames -v`
Expected: PASS.

- [ ] **Step 9: Apply the migration and verify it loads**

Run: `make db-migrate-up` (or the project's migrate target). Expected: migration applies cleanly, no error, and `checkLatestMigrationFile` passes (i.e. `db/migrations.latest` matches the new file's timestamp).

- [ ] **Step 10: Commit**

```bash
git add db/migrations db/migrations.latest pkg/lightwell/db/schema.sql pkg/models/lightwell_package.go pkg/models/repository_configuration.go pkg/models/lightwell_package_test.go
git commit -m "feat: add lightwell package mirror tables and models"
```

---

### Task 2: Shared coords package (BuildPURL / BuildCoordinates)

The handler currently owns unexported `buildPURL`/`buildCoordinates`. The import (Task 6) needs `BuildPURL` and the rewritten handler (Task 8) still needs `BuildCoordinates`. Extract both into a shared package so there is one implementation.

**Files:**
- Create: `pkg/lightwell/coords/coords.go`
- Test: `pkg/lightwell/coords/coords_test.go`

**Interfaces:**
- Produces: `coords.BuildPURL(contentType, group, name, version string) string`, `coords.BuildCoordinates(contentType, group, name string) string`.

- [ ] **Step 1: Write the failing test**

Create `pkg/lightwell/coords/coords_test.go`:

```go
package coords

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
)

func TestBuildPURL(t *testing.T) {
	cases := []struct{ ct, group, name, version, want string }{
		{config.ContentTypeMaven, "org.apache", "commons", "1.0", "pkg:maven/org.apache/commons@1.0"},
		{config.ContentTypePython, "", "requests", "2.0", "pkg:pypi/requests@2.0"},
		{config.ContentTypeNpm, "-", "left-pad", "1.0.0", "pkg:npm/left-pad@1.0.0"},
		{config.ContentTypeNpm, "@types", "node", "20.0.0", "pkg:npm/%40types/node@20.0.0"},
	}
	for _, c := range cases {
		if got := BuildPURL(c.ct, c.group, c.name, c.version); got != c.want {
			t.Errorf("BuildPURL(%q,%q,%q,%q)=%q want %q", c.ct, c.group, c.name, c.version, got, c.want)
		}
	}
}

func TestBuildCoordinates(t *testing.T) {
	cases := []struct{ ct, group, name, want string }{
		{config.ContentTypeMaven, "org.apache", "commons", "org.apache:commons"},
		{config.ContentTypePython, "", "requests", "requests"},
		{config.ContentTypeNpm, "-", "left-pad", "left-pad"},
		{config.ContentTypeNpm, "@types", "node", "@types/node"},
	}
	for _, c := range cases {
		if got := BuildCoordinates(c.ct, c.group, c.name); got != c.want {
			t.Errorf("BuildCoordinates(%q,%q,%q)=%q want %q", c.ct, c.group, c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/lightwell/coords/ -v`
Expected: FAIL (package/functions do not exist).

- [ ] **Step 3: Implement the package**

Create `pkg/lightwell/coords/coords.go` (bodies copied verbatim from the current handler `buildPURL`/`buildCoordinates`):

```go
package coords

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
)

// BuildPURL returns the Package URL for a package version.
func BuildPURL(contentType, group, name, version string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return fmt.Sprintf("pkg:maven/%s/%s@%s", group, name, version)
	case config.ContentTypePython:
		return fmt.Sprintf("pkg:pypi/%s@%s", name, version)
	case config.ContentTypeNpm:
		if group == "-" || group == "" {
			return fmt.Sprintf("pkg:npm/%s@%s", name, version)
		}
		scope := strings.TrimPrefix(group, "@")
		return fmt.Sprintf("pkg:npm/%%40%s/%s@%s", scope, name, version)
	default:
		return ""
	}
}

// BuildCoordinates returns the ecosystem coordinate string for a package.
func BuildCoordinates(contentType, group, name string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return fmt.Sprintf("%s:%s", group, name)
	case config.ContentTypePython:
		return name
	case config.ContentTypeNpm:
		if group == "-" || group == "" {
			return name
		}
		return fmt.Sprintf("%s/%s", group, name)
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/lightwell/coords/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/lightwell/coords/
git commit -m "feat: add shared coords package for purl/coordinates"
```

---

### Task 3: Pulp client GetLatestVersionHref method

The import needs a robust, ecosystem-agnostic way to read a repository's current `latest_version_href` from its repository href (returned by `ResolveRepositoryFromBasePath`). Use the generic `RepositoriesList(...).PulpHrefIn(...)` filter.

**Files:**
- Modify: `pkg/clients/pulp_client/repositories.go`
- Modify: `pkg/clients/pulp_client/interfaces.go` (add to the `PulpClient` interface, near `ResolveRepositoryFromBasePath` at `:107`)
- Modify: `pkg/clients/pulp_client/pulp_client_mock.go` (regenerated)
- Test: `pkg/clients/pulp_client/repositories_test.go` (add a test; create the file if absent)

**Interfaces:**
- Consumes: existing `getZestClient`, `r.domainName`, `errorWithResponseBody` in the package; zest `RepositoriesAPI.RepositoriesList(ctx, domain).PulpHrefIn([]string{href}).Execute()`, `resp.GetResults()`, `RepositoryResponse.GetLatestVersionHref()`.
- Produces: `PulpClient.GetLatestVersionHref(ctx context.Context, repoHref string) (*string, error)` — returns the repository's `latest_version_href`, or `(nil, nil)` when no repository matches the href.

- [ ] **Step 1: Add the method to the interface**

In `pkg/clients/pulp_client/interfaces.go`, add this line to the `PulpClient` interface next to `ResolveRepositoryFromBasePath`:

```go
	GetLatestVersionHref(ctx context.Context, repoHref string) (*string, error)
```

- [ ] **Step 2: Implement the method**

Append to `pkg/clients/pulp_client/repositories.go`:

```go
// GetLatestVersionHref returns the current latest_version_href of a repository,
// looked up generically by its repository href. This value changes whenever the
// repository's content changes, so it is used to detect when a Lightwell repo
// needs re-importing. Returns (nil, nil) if no repository matches the href.
func (r *pulpDaoImpl) GetLatestVersionHref(ctx context.Context, repoHref string) (*string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, httpResp, err := client.RepositoriesAPI.RepositoriesList(ctx, r.domainName).
		PulpHrefIn([]string{repoHref}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error reading repository", httpResp, err)
	}
	results := resp.GetResults()
	if len(results) == 0 {
		return nil, nil
	}
	href := results[0].GetLatestVersionHref()
	return &href, nil
}
```

- [ ] **Step 3: Regenerate the mock and verify compilation**

Run: `make mock`
Then: `go build ./pkg/clients/pulp_client/...`
Expected: builds; `MockPulpClient` now has `GetLatestVersionHref`.

- [ ] **Step 4: Add a mock-based smoke test**

Add to `pkg/clients/pulp_client/repositories_test.go` (matching the package's existing test style; if the file does not exist, create it with the package's suite pattern). Minimal standalone test:

```go
package pulp_client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLatestVersionHrefMockContract(t *testing.T) {
	m := NewMockPulpClient(t)
	href := "/pulp/api/v3/repositories/rpm/rpm/abc/versions/3/"
	m.On("GetLatestVersionHref", context.Background(), "/pulp/api/v3/repositories/rpm/rpm/abc/").
		Return(&href, nil)
	got, err := m.GetLatestVersionHref(context.Background(), "/pulp/api/v3/repositories/rpm/rpm/abc/")
	assert.NoError(t, err)
	assert.Equal(t, href, *got)
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./pkg/clients/pulp_client/ -run TestGetLatestVersionHrefMockContract -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/clients/pulp_client/
git commit -m "feat: add PulpClient.GetLatestVersionHref"
```

---

### Task 4: sqlc read queries + LightwellPackage read DAO

**Files:**
- Create: `pkg/lightwell/db/queries/packages.sql`
- Modify (regenerated): `pkg/lightwell/db/store/models.go`, `pkg/lightwell/db/store/querier.go`, `pkg/lightwell/db/store/packages.sql.go`
- Create: `pkg/dao/lightwell_package.go`
- Modify: `pkg/dao/interfaces.go` (interface + `DaoRegistry` field + `GetDaoRegistry` wiring)
- Modify: `.mockery_v3.yml` (register `LightwellPackageDao`)
- Modify (regenerated): `pkg/dao/dao_mock.go`
- Test: `pkg/dao/lightwell_package_test.go`

**Interfaces:**
- Consumes: `store.Querier` (via `csdb.LightwellQueries`); Task 1 tables.
- Produces:
  - `dao.LightwellPackageDao` interface with `ListPackages(ctx, ListLightwellPackagesOptions) ([]LightwellPackageRow, int64, error)` and `ListPackageVersions(ctx, ListLightwellPackageVersionsOptions) ([]LightwellPackageVersionRow, int64, error)` (the `SyncPackagesForRepository` write method is added in Task 5).
  - Types `ListLightwellPackagesOptions`, `ListLightwellPackageVersionsOptions`, `LightwellPackageRow`, `LightwellPackageVersionRow` (defined below).
  - `DaoRegistry.LightwellPackage LightwellPackageDao`.

- [ ] **Step 1: Write the sqlc queries**

Create `pkg/lightwell/db/queries/packages.sql`:

```sql
-- name: ListLightwellPackages :many
SELECT
    p.uuid,
    p.repository_configuration_uuid,
    rc.name AS repository_name,
    r.content_type AS ecosystem,
    p.name,
    p.package_group,
    array_agg(pv.version ORDER BY pv.version) AS versions,
    array_agg(pv.release ORDER BY pv.version) AS releases,
    array_agg(pv.published_at ORDER BY pv.version) AS published_ats,
    COUNT(*) OVER() AS total_count
FROM lightwell_packages p
JOIN repository_configurations rc ON rc.uuid = p.repository_configuration_uuid
JOIN repositories r ON r.uuid = rc.repository_uuid
LEFT JOIN lightwell_package_versions pv ON pv.lightwell_package_uuid = p.uuid
WHERE r.origin = 'lightwell'
    AND (sqlc.narg(ecosystem)::text IS NULL OR r.content_type = sqlc.narg(ecosystem)::text)
    AND (sqlc.narg(name)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(name)::text || '%')
    AND (sqlc.narg(repository)::text IS NULL OR lower(rc.name) = lower(sqlc.narg(repository)::text))
    AND (sqlc.narg(security_level)::text IS NULL OR lower(r.security_level) = lower(sqlc.narg(security_level)::text))
    AND (
        sqlc.narg(entitled_features)::text[] IS NULL
        OR rc.feature_name IS NULL
        OR btrim(rc.feature_name) = ''
        OR EXISTS (
            SELECT 1
            FROM unnest(string_to_array(rc.feature_name, ',')) AS t(token)
            WHERE btrim(t.token) = ANY(sqlc.narg(entitled_features)::text[])
        )
    )
GROUP BY p.uuid, p.repository_configuration_uuid, rc.name, r.content_type, p.name, p.package_group
ORDER BY p.package_group, p.name
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListLightwellPackageVersions :many
SELECT
    pv.uuid,
    pv.repository_configuration_uuid,
    rc.name AS repository_name,
    r.content_type AS ecosystem,
    p.name,
    p.package_group,
    pv.version,
    pv.release,
    pv.published_at,
    pv.purl,
    COUNT(*) OVER() AS total_count
FROM lightwell_package_versions pv
JOIN lightwell_packages p ON p.uuid = pv.lightwell_package_uuid
JOIN repository_configurations rc ON rc.uuid = pv.repository_configuration_uuid
JOIN repositories r ON r.uuid = rc.repository_uuid
WHERE r.origin = 'lightwell'
    AND (sqlc.narg(ecosystem)::text IS NULL OR r.content_type = sqlc.narg(ecosystem)::text)
    AND (sqlc.narg(name)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(name)::text || '%')
    AND (sqlc.narg(repository)::text IS NULL OR lower(rc.name) = lower(sqlc.narg(repository)::text))
    AND (sqlc.narg(security_level)::text IS NULL OR lower(r.security_level) = lower(sqlc.narg(security_level)::text))
    AND (
        sqlc.narg(entitled_features)::text[] IS NULL
        OR rc.feature_name IS NULL
        OR btrim(rc.feature_name) = ''
        OR EXISTS (
            SELECT 1
            FROM unnest(string_to_array(rc.feature_name, ',')) AS t(token)
            WHERE btrim(t.token) = ANY(sqlc.narg(entitled_features)::text[])
        )
    )
    AND (
        sqlc.narg(resolves_cve_id)::text IS NULL
        OR EXISTS (
            SELECT 1 FROM lightwell_advisories la
            WHERE la.repository_configuration_uuid = rc.uuid
                AND la.package_name = p.name
                AND la.advisory_id = sqlc.narg(resolves_cve_id)::text
                AND pv.version = ANY(la.fixed_versions)
        )
    )
    AND (
        sqlc.narg(vulnerable_to_cve_id)::text IS NULL
        OR (
            EXISTS (
                SELECT 1 FROM lightwell_advisories la
                WHERE la.repository_configuration_uuid = rc.uuid
                    AND la.package_name = p.name
                    AND la.advisory_id = sqlc.narg(vulnerable_to_cve_id)::text
            )
            AND NOT EXISTS (
                SELECT 1 FROM lightwell_advisories la
                WHERE la.repository_configuration_uuid = rc.uuid
                    AND la.package_name = p.name
                    AND la.advisory_id = sqlc.narg(vulnerable_to_cve_id)::text
                    AND pv.version = ANY(la.fixed_versions)
            )
        )
    )
ORDER BY p.package_group, p.name, pv.version
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
```

> `lightwell_advisories` already exists in `schema.sql`. The `array_agg` columns yield `[]string` under the `emit_empty_slices` config; `published_at`/`release` are `NOT NULL` text, so no nullable-array issues.

- [ ] **Step 2: Generate sqlc and verify**

Run: `make sqlc-generate-lightwell`
Then: `go build ./pkg/lightwell/...`
Expected: `store/packages.sql.go` created with `ListLightwellPackages`/`ListLightwellPackageVersions`, their `...Params` and `...Row` types, and `querier.go` updated. Fix any type-override surprises before continuing.

- [ ] **Step 3: Write the DAO read implementation**

Create `pkg/dao/lightwell_package.go`:

```go
package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"gorm.io/gorm"
)

type LightwellPackageRow struct {
	RepositoryConfigurationUUID string
	RepositoryName              string
	Ecosystem                   string
	Name                        string
	Group                       string
	Versions                    []string
	Releases                    []string
	PublishedAts                []string
	TotalCount                  int64
}

type LightwellPackageVersionRow struct {
	RepositoryConfigurationUUID string
	RepositoryName              string
	Ecosystem                   string
	Name                        string
	Group                       string
	Version                     string
	Release                     string
	PublishedAt                 string
	Purl                        string
	TotalCount                  int64
}

type ListLightwellPackagesOptions struct {
	Ecosystem        *string
	Name             *string
	Repository       *string
	SecurityLevel    *string
	EntitledFeatures []string
	Limit            int32
	Offset           int32
}

type ListLightwellPackageVersionsOptions struct {
	Ecosystem         *string
	Name              *string
	Repository        *string
	SecurityLevel     *string
	ResolvesCveID     *string
	VulnerableToCveID *string
	EntitledFeatures  []string
	Limit             int32
	Offset            int32
}

type lightwellPackageDaoImpl struct {
	db      *gorm.DB
	querier store.Querier
}

func GetLightwellPackageDao(db *gorm.DB) LightwellPackageDao {
	return lightwellPackageDaoImpl{db: db}
}

func (d lightwellPackageDaoImpl) ListPackages(ctx context.Context, opts ListLightwellPackagesOptions) ([]LightwellPackageRow, int64, error) {
	if d.querier == nil {
		return nil, 0, fmt.Errorf("lightwell querier is not initialized")
	}
	params := store.ListLightwellPackagesParams{
		Ecosystem:     opts.Ecosystem,
		Name:          opts.Name,
		Repository:    opts.Repository,
		SecurityLevel: opts.SecurityLevel,
		PageLimit:     opts.Limit,
		PageOffset:    opts.Offset,
	}
	if len(opts.EntitledFeatures) > 0 {
		params.EntitledFeatures = opts.EntitledFeatures
	}
	rows, err := d.querier.ListLightwellPackages(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list lightwell packages: %w", err)
	}
	var total int64
	out := make([]LightwellPackageRow, 0, len(rows))
	for _, row := range rows {
		total = row.TotalCount
		out = append(out, LightwellPackageRow{
			RepositoryConfigurationUUID: row.RepositoryConfigurationUuid.String(),
			RepositoryName:              derefString(row.RepositoryName),
			Ecosystem:                   row.Ecosystem,
			Name:                        row.Name,
			Group:                       row.PackageGroup,
			Versions:                    row.Versions,
			Releases:                    row.Releases,
			PublishedAts:                row.PublishedAts,
			TotalCount:                  row.TotalCount,
		})
	}
	return out, total, nil
}

func (d lightwellPackageDaoImpl) ListPackageVersions(ctx context.Context, opts ListLightwellPackageVersionsOptions) ([]LightwellPackageVersionRow, int64, error) {
	if d.querier == nil {
		return nil, 0, fmt.Errorf("lightwell querier is not initialized")
	}
	params := store.ListLightwellPackageVersionsParams{
		Ecosystem:         opts.Ecosystem,
		Name:              opts.Name,
		Repository:        opts.Repository,
		SecurityLevel:     opts.SecurityLevel,
		ResolvesCveID:     opts.ResolvesCveID,
		VulnerableToCveID: opts.VulnerableToCveID,
		PageLimit:         opts.Limit,
		PageOffset:        opts.Offset,
	}
	if len(opts.EntitledFeatures) > 0 {
		params.EntitledFeatures = opts.EntitledFeatures
	}
	rows, err := d.querier.ListLightwellPackageVersions(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list lightwell package versions: %w", err)
	}
	var total int64
	out := make([]LightwellPackageVersionRow, 0, len(rows))
	for _, row := range rows {
		total = row.TotalCount
		out = append(out, LightwellPackageVersionRow{
			RepositoryConfigurationUUID: row.RepositoryConfigurationUuid.String(),
			RepositoryName:              derefString(row.RepositoryName),
			Ecosystem:                   row.Ecosystem,
			Name:                        row.Name,
			Group:                       row.PackageGroup,
			Version:                     row.Version,
			Release:                     row.Release,
			PublishedAt:                 row.PublishedAt,
			Purl:                        row.Purl,
			TotalCount:                  row.TotalCount,
		})
	}
	return out, total, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

> Adjust field access (`row.RepositoryConfigurationUuid`, `row.RepositoryName` pointer-ness, `.String()` on uuid) to match the exact generated `store.ListLightwellPackagesRow` — inspect `store/packages.sql.go` after Step 2. `repository_name` comes from `rc.name` which is nullable in `schema.sql`, so sqlc emits `*string`; hence `derefString`. If `derefString` already exists in the package, reuse it instead of redefining.

- [ ] **Step 4: Wire the interface + registry**

In `pkg/dao/interfaces.go`, add the interface after `LightwellVulnerabilityDao` (near `:294`):

```go
type LightwellPackageDao interface {
	ListPackages(ctx context.Context, opts ListLightwellPackagesOptions) ([]LightwellPackageRow, int64, error)
	ListPackageVersions(ctx context.Context, opts ListLightwellPackageVersionsOptions) ([]LightwellPackageVersionRow, int64, error)
	SyncPackagesForRepository(ctx context.Context, repoConfigUUID string, pkgs []LightwellPackageInput) error
}
```

> `SyncPackagesForRepository` and `LightwellPackageInput` are implemented in Task 5; declaring the method here now means the impl must be added before this file compiles. To keep this task self-contained and compiling, add the `SyncPackagesForRepository` stub to `lightwell_package.go` in this task returning `fmt.Errorf("not implemented")` and a placeholder `LightwellPackageInput` type; Task 5 replaces the stub with the real body and moves/keeps the type. (Alternatively, implement Task 5 immediately after Step 3 here.)

Add the struct field after `LightwellVulnerability` (`:38`):

```go
	LightwellPackage LightwellPackageDao
```

Add the registry wiring after `:86`:

```go
		LightwellPackage: lightwellPackageDaoImpl{db: db, querier: csdb.LightwellQueries},
```

- [ ] **Step 5: Register the mock and regenerate**

In `.mockery_v3.yml`, add under the `pkg/dao` `interfaces:` block (next to `LightwellVulnerabilityDao: {}`):

```yaml
      LightwellPackageDao: {}
```

Run: `make mock`
Then: `go build ./pkg/dao/...`
Expected: `MockLightwellPackageDao` generated in `pkg/dao/dao_mock.go`.

- [ ] **Step 6: Write DB-backed DAO tests**

Create `pkg/dao/lightwell_package_test.go` following the package's existing suite pattern (testify `suite` with a transaction-per-test DB; mirror `lightwell_advisory_test.go`). Seed one Lightwell `repositories` row (origin `lightwell`, content_type maven, security_level `validated`, feature_name matching an entitled token) + its `repository_configurations` row, insert `lightwell_packages`/`lightwell_package_versions`, then assert:

```go
// Representative assertions (fill in suite scaffolding to match lightwell_advisory_test.go):

// 1. Lists packages with aggregated versions and correct total_count.
rows, total, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
	EntitledFeatures: []string{"lightwell-maven"}, Limit: 100, Offset: 0,
})
require.NoError(t, err)
assert.Equal(t, int64(1), total)
assert.ElementsMatch(t, []string{"1.0", "2.0"}, rows[0].Versions)

// 2. Name filter is case-insensitive substring.
rows, _, err = dao.ListPackages(ctx, ListLightwellPackagesOptions{
	Name: ptr("COMM"), EntitledFeatures: []string{"lightwell-maven"}, Limit: 100,
})
require.NoError(t, err)
assert.Len(t, rows, 1)

// 3. security_level filter is case-insensitive equality.
rows, _, err = dao.ListPackages(ctx, ListLightwellPackagesOptions{
	SecurityLevel: ptr("VALIDATED"), EntitledFeatures: []string{"lightwell-maven"}, Limit: 100,
})
require.NoError(t, err)
assert.Len(t, rows, 1)

// 4. Entitlement: non-matching entitled_features excludes the repo's packages.
rows, total, err = dao.ListPackages(ctx, ListLightwellPackagesOptions{
	EntitledFeatures: []string{"some-other-feature"}, Limit: 100,
})
require.NoError(t, err)
assert.Equal(t, int64(0), total)
assert.Empty(t, rows)

// 5. NULL/blank feature_name repo stays visible when entitled_features is provided.
//    (Seed a second repo with feature_name = '' and assert its package appears.)

// 6. ListPackageVersions returns one row per version with purl populated.
vrows, vtotal, err := dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
	EntitledFeatures: []string{"lightwell-maven"}, Limit: 100,
})
require.NoError(t, err)
assert.Equal(t, int64(2), vtotal)
assert.NotEmpty(t, vrows[0].Purl)

// 7. resolves_cve_id: seed a lightwell_advisories row (advisory_id "CVE-1",
//    package_name matching, fixed_versions {"2.0"}); only version 2.0 returned.
vrows, _, err = dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
	ResolvesCveID: ptr("CVE-1"), EntitledFeatures: []string{"lightwell-maven"}, Limit: 100,
})
require.NoError(t, err)
require.Len(t, vrows, 1)
assert.Equal(t, "2.0", vrows[0].Version)

// 8. vulnerable_to_cve_id: same advisory; only NON-fixed version 1.0 returned.
vrows, _, err = dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
	VulnerableToCveID: ptr("CVE-1"), EntitledFeatures: []string{"lightwell-maven"}, Limit: 100,
})
require.NoError(t, err)
require.Len(t, vrows, 1)
assert.Equal(t, "1.0", vrows[0].Version)
```

Add a local `func ptr(s string) *string { return &s }` if the package lacks one.

- [ ] **Step 7: Run the tests**

Run: `go test ./pkg/dao/ -run LightwellPackage -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/lightwell/db pkg/dao/lightwell_package.go pkg/dao/lightwell_package_test.go pkg/dao/interfaces.go pkg/dao/dao_mock.go .mockery_v3.yml
git commit -m "feat: add lightwell package read DAO and sqlc queries"
```

---

### Task 5: LightwellPackage write DAO + repo-config import helpers

**Files:**
- Modify: `pkg/dao/lightwell_package.go` (add `SyncPackagesForRepository` + input types)
- Modify: `pkg/dao/interfaces.go` (add two methods to `RepositoryConfigDao`; `SyncPackagesForRepository` already declared in Task 4)
- Modify: `pkg/dao/repository_configs.go` (implement the two internal methods)
- Modify (regenerated): `pkg/dao/dao_mock.go`
- Test: `pkg/dao/lightwell_package_test.go` (add write tests)
- Test: `pkg/dao/repository_configs_test.go` (add import-helper tests)

**Interfaces:**
- Produces:
  - `dao.LightwellPackageInput{ Name, Group string; Versions []LightwellPackageVersionInput }`
  - `dao.LightwellPackageVersionInput{ Version, Release, PublishedAt, Purl string }`
  - `LightwellPackageDao.SyncPackagesForRepository(ctx, repoConfigUUID string, pkgs []LightwellPackageInput) error`
  - `RepositoryConfigDao.InternalOnly_ListLightwellReposToImport(ctx) ([]LightwellRepoToImport, error)` and `RepositoryConfigDao.InternalOnly_UpdateLastImportRepositoryVersion(ctx, repoConfigUUID, versionHref string) error`
  - `dao.LightwellRepoToImport{ RepoConfigUUID, OrgID, Name, ContentType, BasePath, LastImportRepositoryVersion string }`

- [ ] **Step 1: Write the failing write-DAO test**

Add to `pkg/dao/lightwell_package_test.go`:

```go
func (s *LightwellPackageSuite) TestSyncPackagesForRepositoryUpsertAndDeleteDiff() {
	t := s.T()
	ctx := context.Background()
	dao := GetLightwellPackageDao(s.tx) // s.tx is the suite's *gorm.DB
	rcUUID := s.seedLightwellRepoConfig("maven", "validated", "lightwell-maven") // helper seeds repo + repo_config

	// initial import: two packages
	err := dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.0", Release: "", PublishedAt: "2020-01-01T00:00:00Z", Purl: "pkg:maven/org.apache/commons@1.0"},
			{Version: "2.0", Purl: "pkg:maven/org.apache/commons@2.0"},
		}},
		{Name: "logging", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.5", Purl: "pkg:maven/org.apache/logging@1.5"},
		}},
	})
	require.NoError(t, err)

	var pkgCount, verCount int64
	s.tx.Model(&models.LightwellPackage{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&pkgCount)
	s.tx.Model(&models.LightwellPackageVersion{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&verCount)
	assert.Equal(t, int64(2), pkgCount)
	assert.Equal(t, int64(3), verCount)

	// capture a stable uuid to prove upsert keeps it
	var before models.LightwellPackage
	s.tx.Where("repository_configuration_uuid = ? AND name = ?", rcUUID, "commons").First(&before)

	// second import: "logging" removed, "commons" drops 1.0, 2.0 purl updated
	err = dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "2.0", Purl: "pkg:maven/org.apache/commons@2.0-updated"},
		}},
	})
	require.NoError(t, err)

	s.tx.Model(&models.LightwellPackage{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&pkgCount)
	s.tx.Model(&models.LightwellPackageVersion{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&verCount)
	assert.Equal(t, int64(1), pkgCount) // logging deleted
	assert.Equal(t, int64(1), verCount) // only commons 2.0 remains

	var after models.LightwellPackage
	s.tx.Where("repository_configuration_uuid = ? AND name = ?", rcUUID, "commons").First(&after)
	assert.Equal(t, before.UUID, after.UUID) // upsert kept the uuid

	var ver models.LightwellPackageVersion
	s.tx.Where("lightwell_package_uuid = ?", after.UUID).First(&ver)
	assert.Equal(t, "pkg:maven/org.apache/commons@2.0-updated", ver.Purl) // updated in place
}

func (s *LightwellPackageSuite) TestSyncPackagesEmptyDeletesAll() {
	t := s.T()
	ctx := context.Background()
	dao := GetLightwellPackageDao(s.tx)
	rcUUID := s.seedLightwellRepoConfig("python", "validated", "lightwell-python")
	require.NoError(t, dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "requests", Group: "", Versions: []LightwellPackageVersionInput{{Version: "2.0", Purl: "pkg:pypi/requests@2.0"}}},
	}))
	require.NoError(t, dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{}))
	var pkgCount int64
	s.tx.Model(&models.LightwellPackage{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&pkgCount)
	assert.Equal(t, int64(0), pkgCount)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/dao/ -run TestSyncPackages -v`
Expected: FAIL (`SyncPackagesForRepository` is the not-implemented stub / types missing).

- [ ] **Step 3: Implement the write method + input types**

In `pkg/dao/lightwell_package.go`, add the imports `"github.com/content-services/content-sources-backend/pkg/models"`, `"gorm.io/gorm/clause"`, and replace the Task-4 stub with:

```go
type LightwellPackageVersionInput struct {
	Version     string
	Release     string
	PublishedAt string
	Purl        string
}

type LightwellPackageInput struct {
	Name     string
	Group    string
	Versions []LightwellPackageVersionInput
}

func (d lightwellPackageDaoImpl) SyncPackagesForRepository(ctx context.Context, repoConfigUUID string, pkgs []LightwellPackageInput) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keepPkgUUIDs := make([]string, 0, len(pkgs))
		keepVerUUIDs := make([]string, 0)

		for _, in := range pkgs {
			pkg := models.LightwellPackage{
				RepositoryConfigurationUUID: repoConfigUUID,
				Name:                        in.Name,
				Group:                       in.Group,
			}
			if err := tx.Clauses(
				clause.OnConflict{
					Columns:   []clause.Column{{Name: "repository_configuration_uuid"}, {Name: "package_group"}, {Name: "name"}},
					DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
				},
				clause.Returning{Columns: []clause.Column{{Name: "uuid"}}},
			).Create(&pkg).Error; err != nil {
				return err
			}
			keepPkgUUIDs = append(keepPkgUUIDs, pkg.UUID)

			for _, v := range in.Versions {
				ver := models.LightwellPackageVersion{
					LightwellPackageUUID:        pkg.UUID,
					RepositoryConfigurationUUID: repoConfigUUID,
					Version:                     v.Version,
					Release:                     v.Release,
					PublishedAt:                 v.PublishedAt,
					Purl:                        v.Purl,
				}
				if err := tx.Clauses(
					clause.OnConflict{
						Columns:   []clause.Column{{Name: "lightwell_package_uuid"}, {Name: "version"}},
						DoUpdates: clause.AssignmentColumns([]string{"release", "published_at", "purl", "updated_at"}),
					},
					clause.Returning{Columns: []clause.Column{{Name: "uuid"}}},
				).Create(&ver).Error; err != nil {
					return err
				}
				keepVerUUIDs = append(keepVerUUIDs, ver.UUID)
			}
		}

		// Delete versions that are no longer present for this repo.
		vq := tx.Where("repository_configuration_uuid = ?", repoConfigUUID)
		if len(keepVerUUIDs) > 0 {
			vq = vq.Where("uuid NOT IN ?", keepVerUUIDs)
		}
		if err := vq.Delete(&models.LightwellPackageVersion{}).Error; err != nil {
			return err
		}

		// Delete packages that are no longer present for this repo.
		pq := tx.Where("repository_configuration_uuid = ?", repoConfigUUID)
		if len(keepPkgUUIDs) > 0 {
			pq = pq.Where("uuid NOT IN ?", keepPkgUUIDs)
		}
		if err := pq.Delete(&models.LightwellPackage{}).Error; err != nil {
			return err
		}
		return nil
	})
}
```

> Remove the Task-4 placeholder `LightwellPackageInput` type/stub if you added one there.

- [ ] **Step 4: Run the write tests**

Run: `go test ./pkg/dao/ -run TestSyncPackages -v`
Expected: PASS.

- [ ] **Step 5: Write the failing repo-config helper test**

Add to `pkg/dao/repository_configs_test.go` a test that seeds two Lightwell repo configs (org `config.LightwellOrg`, content types maven and python) and one non-Lightwell repo config, then:

```go
repos, err := dao.InternalOnly_ListLightwellReposToImport(ctx)
require.NoError(t, err)
// only the two lightwell maven/python repos, each with BasePath + ContentType populated
assert.Len(t, repos, 2)

// update + read back
err = dao.InternalOnly_UpdateLastImportRepositoryVersion(ctx, repos[0].RepoConfigUUID, "/versions/5/")
require.NoError(t, err)
var rc models.RepositoryConfiguration
tx.Where("uuid = ?", repos[0].RepoConfigUUID).First(&rc)
assert.Equal(t, "/versions/5/", rc.LastImportRepositoryVersion)
```

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./pkg/dao/ -run InternalOnly_ListLightwellReposToImport -v`
Expected: FAIL (methods missing).

- [ ] **Step 7: Declare and implement the repo-config helpers**

In `pkg/dao/interfaces.go`, add to the `RepositoryConfigDao` interface:

```go
	InternalOnly_ListLightwellReposToImport(ctx context.Context) ([]LightwellRepoToImport, error)
	InternalOnly_UpdateLastImportRepositoryVersion(ctx context.Context, repoConfigUUID string, versionHref string) error
```

Add the type near the other DAO types in `interfaces.go`:

```go
type LightwellRepoToImport struct {
	RepoConfigUUID              string
	OrgID                       string
	Name                        string
	ContentType                 string
	BasePath                    string
	LastImportRepositoryVersion string
}
```

Implement in `pkg/dao/repository_configs.go`:

```go
func (r repositoryConfigDaoImpl) InternalOnly_ListLightwellReposToImport(ctx context.Context) ([]LightwellRepoToImport, error) {
	var configs []models.RepositoryConfiguration
	result := r.db.WithContext(ctx).
		Preload("Repository").
		Joins("JOIN repositories ON repositories.uuid = repository_configurations.repository_uuid").
		Where("repository_configurations.org_id IN ?", []string{config.LightwellOrg, config.LightwellDemoOrg}).
		Where("repositories.content_type IN ?", []string{config.ContentTypeMaven, config.ContentTypePython, config.ContentTypeNpm}).
		Find(&configs)
	if result.Error != nil {
		return nil, result.Error
	}
	out := make([]LightwellRepoToImport, 0, len(configs))
	for _, c := range configs {
		out = append(out, LightwellRepoToImport{
			RepoConfigUUID:              c.UUID,
			OrgID:                       c.OrgID,
			Name:                        c.Name,
			ContentType:                 c.Repository.ContentType,
			BasePath:                    c.Repository.PublishedDistBasePath,
			LastImportRepositoryVersion: c.LastImportRepositoryVersion,
		})
	}
	return out, nil
}

func (r repositoryConfigDaoImpl) InternalOnly_UpdateLastImportRepositoryVersion(ctx context.Context, repoConfigUUID string, versionHref string) error {
	return r.db.WithContext(ctx).
		Model(&models.RepositoryConfiguration{}).
		Where("uuid = ?", repoConfigUUID).
		Update("last_import_repository_version", versionHref).Error
}
```

- [ ] **Step 8: Regenerate mocks + build**

Run: `make mock` then `go build ./...`
Expected: `MockRepositoryConfigDao` and `MockLightwellPackageDao` updated; whole module builds.

- [ ] **Step 9: Run all new DAO tests**

Run: `go test ./pkg/dao/ -run 'LightwellPackage|InternalOnly_ListLightwellReposToImport|TestSyncPackages' -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add pkg/dao/ .mockery_v3.yml
git commit -m "feat: add lightwell package write DAO and repo import helpers"
```

---

### Task 6: Import command + CLI registration

**Files:**
- Create: `pkg/external_repos/commands/import_lightwell_packages.go`
- Modify: `cmd/external-repos/main.go` (register the command in `Commands`)
- Test: `pkg/external_repos/commands/import_lightwell_packages_test.go`

**Interfaces:**
- Consumes: `dao.RepositoryConfigDao` helpers (Task 5), `dao.LightwellPackageDao.SyncPackagesForRepository` (Task 5), `dao.Domain.FetchOrCreateDomain`, `pulp_client.GetPulpClientWithDomain`, `pulp_client.PulpClient.{ResolveRepositoryFromBasePath,GetLatestVersionHref,ListMavenPackages}`, `config.Tang` (`config.ConfigureTang`), `coords.BuildPURL`, `handler.ParseNpmPackageName`.
- Produces: `commands.ImportLightwellPackagesAction(c *cli.Context) error`; pure mapping helpers `mapMavenPackageInputs`, `mapPythonPackageInputs`, `mapNpmPackageInputs` returning `[]dao.LightwellPackageInput`; `shouldImport(current *string, last string, force bool) bool`.

- [ ] **Step 1: Write failing unit tests for the pure helpers**

Create `pkg/external_repos/commands/import_lightwell_packages_test.go`:

```go
package commands

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
)

func TestShouldImport(t *testing.T) {
	v := "/versions/5/"
	assert.True(t, shouldImport(&v, "/versions/4/", false))  // changed
	assert.False(t, shouldImport(&v, "/versions/5/", false)) // unchanged
	assert.True(t, shouldImport(&v, "/versions/5/", true))   // force
	assert.False(t, shouldImport(nil, "/versions/5/", false)) // nil current -> nothing to import
}

func TestMapMavenPackageInputs(t *testing.T) {
	resp := zest.PaginatedMavenRepositoryPackageListResponse{
		Results: []zest.MavenRepositoryPackageResponse{{
			GroupId:    "org.apache",
			ArtifactId: "commons",
			Versions:   []string{"1.0", "2.0"},
			LatestReleases: []zest.MavenPackageReleaseResponse{
				{Version: "2.0", Release: "ga", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		}},
	}
	got := mapMavenPackageInputs(resp)
	assert.Len(t, got, 1)
	assert.Equal(t, "commons", got[0].Name)
	assert.Equal(t, "org.apache", got[0].Group)
	assert.Len(t, got[0].Versions, 2)
	// version 2.0 carries release + published_at + purl; 1.0 has empty release/published_at
	byVer := map[string]dao_LightwellPackageVersionInput(got[0].Versions)
	assert.Equal(t, "ga", byVer["2.0"].Release)
	assert.Equal(t, "2020-01-01T00:00:00Z", byVer["2.0"].PublishedAt)
	assert.Equal(t, "pkg:maven/org.apache/commons@2.0", byVer["2.0"].Purl)
	assert.Equal(t, "", byVer["1.0"].Release)
}

func TestMapPythonPackageInputs(t *testing.T) {
	resp := tangy.PythonPackageListResponse{
		Results: []tangy.PythonPackageListItem{{
			NameNormalized: "requests",
			Versions:       []string{"2.0"},
			LatestVersions: []tangy.PythonVersionInfo{{Version: "2.0", CreatedAt: "2021-01-01T00:00:00Z"}},
		}},
	}
	got := mapPythonPackageInputs(resp)
	assert.Len(t, got, 1)
	assert.Equal(t, "requests", got[0].Name)
	assert.Equal(t, "", got[0].Group)
	assert.Equal(t, "pkg:pypi/requests@2.0", got[0].Versions[0].Purl)
	assert.Equal(t, "2021-01-01T00:00:00Z", got[0].Versions[0].PublishedAt)
}

func TestMapNpmPackageInputs(t *testing.T) {
	resp := tangy.NpmPackageListResponse{
		Results: []tangy.NpmPackageListItem{{
			Name:           "@types/node",
			Versions:       []string{"20.0.0"},
			LatestVersions: []tangy.NpmVersionInfo{{Version: "20.0.0", CreatedAt: "2022-01-01T00:00:00Z"}},
		}},
	}
	got := mapNpmPackageInputs(resp)
	assert.Len(t, got, 1)
	assert.Equal(t, "node", got[0].Name)
	assert.Equal(t, "@types", got[0].Group)
	assert.Equal(t, "pkg:npm/%40types/node@20.0.0", got[0].Versions[0].Purl)
	_ = config.ContentTypeNpm
}
```

> The `dao_LightwellPackageVersionInput` alias in the test is shorthand — replace with a small inline loop building `map[string]dao.LightwellPackageVersionInput` from `got[0].Versions`. (Written out fully in the implementer's test; kept short here.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/external_repos/commands/ -run 'TestShouldImport|TestMap.*PackageInputs' -v`
Expected: FAIL (functions undefined).

- [ ] **Step 3: Implement the command**

Create `pkg/external_repos/commands/import_lightwell_packages.go`:

```go
package commands

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/lightwell/coords"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

const importMaxConcurrentRepos = 10
const importPageSize = 300

func ImportLightwellPackagesAction(c *cli.Context) error {
	ctx := c.Context
	force := c.Bool("force")
	if config.Tang == nil {
		if err := config.ConfigureTang(); err != nil {
			return fmt.Errorf("failed to configure tang: %w", err)
		}
	}
	if err := importLightwellPackages(ctx, db.DB, force); err != nil {
		log.Error().Err(err).Msg("Failed to import lightwell packages")
		return err
	}
	log.Info().Msg("Successfully imported lightwell packages.")
	return nil
}

func importLightwellPackages(ctx context.Context, database *gorm.DB, force bool) error {
	daoReg := dao.GetDaoRegistry(database)
	repos, err := daoReg.RepositoryConfig.InternalOnly_ListLightwellReposToImport(ctx)
	if err != nil {
		return fmt.Errorf("error listing lightwell repos: %w", err)
	}

	errsByIdx := make([]error, len(repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, importMaxConcurrentRepos)

	for i, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, r dao.LightwellRepoToImport) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := importRepo(ctx, daoReg, r, force); err != nil {
				log.Error().Err(err).Str("repo", r.Name).Msg("Failed to import lightwell repo, continuing")
				errsByIdx[idx] = fmt.Errorf("repo %s: %w", r.Name, err)
			}
		}(i, repo)
	}
	wg.Wait()

	var errs []error
	for _, e := range errsByIdx {
		if e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

func importRepo(ctx context.Context, daoReg *dao.DaoRegistry, repo dao.LightwellRepoToImport, force bool) error {
	if repo.BasePath == "" {
		return nil
	}
	domain, err := daoReg.Domain.FetchOrCreateDomain(ctx, repo.OrgID)
	if err != nil {
		return err
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domain)
	repoHref, err := pulpClient.ResolveRepositoryFromBasePath(ctx, repo.BasePath)
	if err != nil {
		return err
	}
	if repoHref == nil {
		log.Warn().Str("repo", repo.Name).Msg("distribution not found, skipping")
		return nil
	}
	currentVersion, err := pulpClient.GetLatestVersionHref(ctx, *repoHref)
	if err != nil {
		return err
	}
	if !shouldImport(currentVersion, repo.LastImportRepositoryVersion, force) {
		return nil
	}

	pkgs, err := fetchRepoPackages(ctx, pulpClient, repo.ContentType, *repoHref)
	if err != nil {
		return err
	}
	if err := daoReg.LightwellPackage.SyncPackagesForRepository(ctx, repo.RepoConfigUUID, pkgs); err != nil {
		return err
	}
	return daoReg.RepositoryConfig.InternalOnly_UpdateLastImportRepositoryVersion(ctx, repo.RepoConfigUUID, *currentVersion)
}

func shouldImport(current *string, last string, force bool) bool {
	if current == nil {
		return false
	}
	if force {
		return true
	}
	return *current != last
}

func fetchRepoPackages(ctx context.Context, pulpClient pulp_client.PulpClient, contentType, repoHref string) ([]dao.LightwellPackageInput, error) {
	switch contentType {
	case config.ContentTypeMaven:
		var all []dao.LightwellPackageInput
		offset := 0
		for {
			resp, err := pulpClient.ListMavenPackages(ctx, repoHref, "", importPageSize, offset)
			if err != nil {
				return nil, err
			}
			all = append(all, mapMavenPackageInputs(resp)...)
			offset += len(resp.Results)
			if len(resp.Results) == 0 || int64(offset) >= resp.Count {
				break
			}
		}
		return all, nil
	case config.ContentTypePython:
		if config.Tang == nil {
			return nil, fmt.Errorf("tang is not configured")
		}
		var all []dao.LightwellPackageInput
		offset := 0
		for {
			resp, err := (*config.Tang).PythonPackageList(ctx, repoHref, tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: offset, Limit: importPageSize})
			if err != nil {
				return nil, err
			}
			all = append(all, mapPythonPackageInputs(resp)...)
			offset += len(resp.Results)
			if len(resp.Results) == 0 || offset >= resp.Total {
				break
			}
		}
		return all, nil
	case config.ContentTypeNpm:
		if config.Tang == nil {
			return nil, fmt.Errorf("tang is not configured")
		}
		var all []dao.LightwellPackageInput
		offset := 0
		for {
			resp, err := (*config.Tang).NpmPackageList(ctx, repoHref, tangy.NpmPackageListFilters{}, tangy.PageOptions{Offset: offset, Limit: importPageSize})
			if err != nil {
				return nil, err
			}
			all = append(all, mapNpmPackageInputs(resp)...)
			offset += len(resp.Results)
			if len(resp.Results) == 0 || offset >= resp.Total {
				break
			}
		}
		return all, nil
	default:
		return nil, nil
	}
}

func mapMavenPackageInputs(resp zest.PaginatedMavenRepositoryPackageListResponse) []dao.LightwellPackageInput {
	out := make([]dao.LightwellPackageInput, 0, len(resp.Results))
	for _, item := range resp.Results {
		rel := make(map[string]zest.MavenPackageReleaseResponse, len(item.LatestReleases))
		for _, r := range item.LatestReleases {
			rel[r.Version] = r
		}
		versions := make([]dao.LightwellPackageVersionInput, 0, len(item.Versions))
		for _, v := range item.Versions {
			in := dao.LightwellPackageVersionInput{
				Version: v,
				Purl:    coords.BuildPURL(config.ContentTypeMaven, item.GroupId, item.ArtifactId, v),
			}
			if r, ok := rel[v]; ok {
				in.Release = r.Release
				in.PublishedAt = r.CreatedAt.Format(time.RFC3339)
			}
			versions = append(versions, in)
		}
		out = append(out, dao.LightwellPackageInput{Name: item.ArtifactId, Group: item.GroupId, Versions: versions})
	}
	return out
}

func mapPythonPackageInputs(resp tangy.PythonPackageListResponse) []dao.LightwellPackageInput {
	out := make([]dao.LightwellPackageInput, 0, len(resp.Results))
	for _, item := range resp.Results {
		created := make(map[string]string, len(item.LatestVersions))
		for _, v := range item.LatestVersions {
			created[v.Version] = v.CreatedAt
		}
		versions := make([]dao.LightwellPackageVersionInput, 0, len(item.Versions))
		for _, v := range item.Versions {
			versions = append(versions, dao.LightwellPackageVersionInput{
				Version:     v,
				PublishedAt: created[v],
				Purl:        coords.BuildPURL(config.ContentTypePython, "", item.NameNormalized, v),
			})
		}
		out = append(out, dao.LightwellPackageInput{Name: item.NameNormalized, Group: "", Versions: versions})
	}
	return out
}

func mapNpmPackageInputs(resp tangy.NpmPackageListResponse) []dao.LightwellPackageInput {
	out := make([]dao.LightwellPackageInput, 0, len(resp.Results))
	for _, item := range resp.Results {
		scope, name := handler.ParseNpmPackageName(item.Name)
		created := make(map[string]string, len(item.LatestVersions))
		for _, v := range item.LatestVersions {
			created[v.Version] = v.CreatedAt
		}
		versions := make([]dao.LightwellPackageVersionInput, 0, len(item.Versions))
		for _, v := range item.Versions {
			versions = append(versions, dao.LightwellPackageVersionInput{
				Version:     v,
				PublishedAt: created[v],
				Purl:        coords.BuildPURL(config.ContentTypeNpm, scope, name, v),
			})
		}
		out = append(out, dao.LightwellPackageInput{Name: name, Group: scope, Versions: versions})
	}
	return out
}
```

> If importing `pkg/handler` from `pkg/external_repos/commands` creates an import cycle, move `ParseNpmPackageName` into `pkg/lightwell/coords` (as `coords.ParseNpmPackageName`) in Task 2 and call it from both the handler and here. Check with `go build ./...` in Step 5 and refactor if needed.

- [ ] **Step 4: Register the CLI command**

In `cmd/external-repos/main.go`, add to the `Commands` slice (after the `sync-lightwell-advisories` block, before the slice closes at `:126`):

```go
			{
				Name:   "import-lightwell-packages",
				Usage:  "Import lightwell package and version data into the database mirror",
				Action: commands.ImportLightwellPackagesAction,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Re-import all lightwell repos regardless of repository version",
					},
				},
			},
```

- [ ] **Step 5: Build and run helper tests**

Run: `go build ./...`
Then: `go test ./pkg/external_repos/commands/ -run 'TestShouldImport|TestMap.*PackageInputs' -v`
Expected: PASS.

- [ ] **Step 6: Add an orchestration test with mocked deps**

Add a test that builds a `dao.DaoRegistry` populated with `dao.MockRepositoryConfigDao`, `dao.MockDomainDao`, and `dao.MockLightwellPackageDao`, plus a `pulp_client.MockPulpClient`, and exercises `importRepo` for the change / no-change / unresolved cases. To make `importRepo` testable without the `GetPulpClientWithDomain` global, refactor `importRepo` to accept the `pulp_client.PulpClient` as a parameter (obtain it in `importLightwellPackages`'s goroutine via `GetPulpClientWithDomain`, pass it down). Example assertions:

```go
// no-change: current version equals stored -> Sync/Update NOT called
// change: current != stored -> Sync called with mapped pkgs, Update called with current
// unresolved: ResolveRepositoryFromBasePath returns (nil,nil) -> skip, no error, no Sync
```

Wire the mocks' `.On(...)` expectations accordingly and assert with `mock.AssertExpectations(t)`.

- [ ] **Step 7: Run the orchestration test**

Run: `go test ./pkg/external_repos/commands/ -run TestImportRepo -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/external_repos/commands/import_lightwell_packages.go pkg/external_repos/commands/import_lightwell_packages_test.go cmd/external-repos/main.go
git commit -m "feat: add import-lightwell-packages command"
```

---

### Task 7: Deployment CronJob (every 15 minutes)

**Files:**
- Modify: `deployments/deployment.yaml` (add a job under `objects[].spec.jobs` and two `parameters`)

**Interfaces:**
- Consumes: the `import-lightwell-packages` CLI command (Task 6).
- Produces: a Clowder CronJob running `/external-repos import-lightwell-packages` every 15 minutes, gated by suspend/schedule params.

- [ ] **Step 1: Add the CronJob object**

In `deployments/deployment.yaml`, immediately after the `sync-lightwell-advisories` job block (ends ~`:2913`), add a sibling job. Copy the env block from `sync-lightwell-advisories` verbatim (it needs the same DB/Pulp/feature-service env), changing only `name`, `schedule`, `suspend`, and the second `command` arg:

```yaml
        - name: import-lightwell-packages
          schedule: ${IMPORT_LIGHTWELL_PACKAGES_CRON_JOB}
          suspend: ${{SUSPEND_IMPORT_LIGHTWELL_PACKAGES}}
          concurrencyPolicy: "Forbid"
          podSpec:
            securityContext:
              runAsNonRoot: true
              runAsUser: 1001
            image: ${IMAGE}:${IMAGE_TAG}
            inheritEnv: true
            command:
              - /external-repos
              - import-lightwell-packages
            env:
              # --- copy the FULL env: block from the sync-lightwell-advisories job here verbatim ---
```

> The env block is large (~lines 2620-2913 in the advisory job). Copy it exactly so the import job has identical DB / Pulp / feature-service / Clowder configuration.

- [ ] **Step 2: Add the parameters**

In the `parameters:` section (near `:7023`, beside `SYNC_LIGHTWELL_ADVISORIES_CRON_JOB`), add:

```yaml
  - name: IMPORT_LIGHTWELL_PACKAGES_CRON_JOB
    value: '*/15 * * * *'
  - name: SUSPEND_IMPORT_LIGHTWELL_PACKAGES
    description: whether to suspend the import-lightwell-packages cronjob
    required: false
    value: 'false'
```

- [ ] **Step 3: Validate YAML**

Run: `python3 -c "import yaml,sys; list(yaml.safe_load_all(open('deployments/deployment.yaml')))" && echo OK`
Expected: `OK` (no parse error). If the repo has a template-lint/`make` target for deployment templates, run it too.

- [ ] **Step 4: Commit**

```bash
git add deployments/deployment.yaml
git commit -m "feat: add import-lightwell-packages cronjob every 15m"
```

---

### Task 8: Rewrite the read endpoints to use the mirror

Convert `ListPackages` / `ListPackageVersions` to query the DAO, and delete the live Pulp/Tang aggregation code paths. Both the main-API registration (`RegisterLightwellPackageRoutes`) and the v0.1 wrappers use this same handler, so they switch together.

**Files:**
- Modify: `pkg/handler/lightwell_packages.go`
- Test: `pkg/handler/lightwell_packages_test.go` (create/extend)

**Interfaces:**
- Consumes: `dao.LightwellPackageDao.ListPackages` / `ListPackageVersions`, `feature_service_client` entitled features via the existing `DaoRegistry` path, `coords.BuildCoordinates`.
- Produces: unchanged HTTP responses (`api.LightwellPackageCollectionResponse`, `api.LightwellPackageVersionCollectionResponse`).

- [ ] **Step 1: Write failing handler tests (mock DAO)**

Create/extend `pkg/handler/lightwell_packages_test.go`. Build the handler with a `dao.MockLightwellPackageDao` and assert:

```go
// GET /lightwell/packages returns rows from the DAO, mapped to responses,
// with Meta.Count from the DAO total; DAO called with parsed filters + entitled features.
// GET /lightwell/package_versions maps versions incl. Coordinates computed via coords.BuildCoordinates
// and Purl passed through; ResolvesCveID/VulnerableToCveID forwarded to the DAO options.
// Neither endpoint calls TangClient or PulpClient (assert those mocks get no calls).
```

Follow the existing handler test harness in `pkg/handler` (echo context + `serveRouter`/equivalent used by sibling handler tests).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/handler/ -run Lightwell -v`
Expected: FAIL (handler still calls Pulp/Tang).

- [ ] **Step 3: Rewrite `ListPackages`**

Replace the body of `ListPackages` (`:57-97`) with a DAO-backed version. It resolves entitled features the same way `RepositoryConfig.List` did — via the feature service. Reuse the entitled-features lookup already available through the registry (the DAO layer accepts them as `EntitledFeatures`). Obtain them from the feature-service client the handler already has access to (mirror how other handlers fetch entitled features), then:

```go
func (h *LightwellPackagesHandler) ListPackages(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageFilters(c)
	if err := validateContentType(filters.Ecosystem); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid ecosystem filter", err.Error())
	}
	_, orgID := GetAccountIdOrgId(c)
	ctx := c.Request().Context()
	entitled, err := h.DaoRegistry.FeatureServiceEntitledFeatures(ctx, orgID) // see Step 5 note
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch entitled features")
	}
	rows, total, err := h.DaoRegistry.LightwellPackage.ListPackages(ctx, dao.ListLightwellPackagesOptions{
		Ecosystem:        optStr(filters.Ecosystem),
		Name:             optStr(filters.Name),
		Repository:       optStr(filters.Repository),
		SecurityLevel:    optStr(filters.SecurityLevel),
		EntitledFeatures: entitled,
		Limit:            int32(page.Limit),
		Offset:           int32(page.Offset),
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}
	data := make([]api.LightwellPackageResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, mapRowToLightwellPackage(row))
	}
	resp := api.LightwellPackageCollectionResponse{Data: data}
	collResp := SetCollectionResponseMetadata(&resp, c, total)
	return c.JSON(http.StatusOK, collResp)
}
```

Add mapping + helper functions:

```go
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mapRowToLightwellPackage(row dao.LightwellPackageRow) api.LightwellPackageResponse {
	releases := make([]api.ReleaseInfo, 0, len(row.Versions))
	for i, v := range row.Versions {
		ri := api.ReleaseInfo{Version: v}
		if i < len(row.Releases) {
			ri.Release = row.Releases[i]
		}
		if i < len(row.PublishedAts) {
			ri.CreatedAt = row.PublishedAts[i]
		}
		releases = append(releases, ri)
	}
	return api.LightwellPackageResponse{
		Name:           row.Name,
		Group:          row.Group,
		Ecosystem:      row.Ecosystem,
		Repository:     row.RepositoryName,
		RepositoryUUID: row.RepositoryConfigurationUUID,
		Versions:       row.Versions,
		LatestReleases: releases,
	}
}
```

- [ ] **Step 4: Rewrite `ListPackageVersions`**

Replace the body of `ListPackageVersions` (`:175-215`):

```go
func (h *LightwellPackagesHandler) ListPackageVersions(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageVersionFilters(c)
	if err := validateContentType(filters.Ecosystem); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid ecosystem filter", err.Error())
	}
	_, orgID := GetAccountIdOrgId(c)
	ctx := c.Request().Context()
	entitled, err := h.DaoRegistry.FeatureServiceEntitledFeatures(ctx, orgID)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch entitled features")
	}
	rows, total, err := h.DaoRegistry.LightwellPackage.ListPackageVersions(ctx, dao.ListLightwellPackageVersionsOptions{
		Ecosystem:         optStr(filters.Ecosystem),
		Name:              optStr(filters.Name),
		Repository:        optStr(filters.Repository),
		SecurityLevel:     optStr(filters.SecurityLevel),
		ResolvesCveID:     optStr(filters.ResolvesCveID),
		VulnerableToCveID: optStr(filters.VulnerableToCveID),
		EntitledFeatures:  entitled,
		Limit:             int32(page.Limit),
		Offset:            int32(page.Offset),
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}
	data := make([]api.LightwellPackageVersionResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, api.LightwellPackageVersionResponse{
			Name:           row.Name,
			Group:          row.Group,
			Version:        row.Version,
			Ecosystem:      row.Ecosystem,
			Repository:     row.RepositoryName,
			RepositoryUUID: row.RepositoryConfigurationUUID,
			Release:        row.Release,
			CreatedAt:      row.PublishedAt,
			Purl:           row.Purl,
			Coordinates:    coords.BuildCoordinates(row.Ecosystem, row.Group, row.Name),
		})
	}
	resp := api.LightwellPackageVersionCollectionResponse{Data: data}
	collResp := SetCollectionResponseMetadata(&resp, c, total)
	return c.JSON(http.StatusOK, collResp)
}
```

- [ ] **Step 5: Delete dead code and fix imports**

Delete the now-unused functions from `lightwell_packages.go`: `fetchLightwellRepos`, `fetchPackagesPage`, `tangSortBy`, `repoPackageResult`, `aggregatePackages`, `fetchAllPages`, `fetchPackagesFromRepo`, `repoVersionResult`, `aggregatePackageVersions`, `fetchVersionsFromRepo`, `resolveRepository`, `filterVersionsByResolvingCve`, `filterVersionsByVulnerableCve`, `buildPURL`, `buildCoordinates`, all `map*ToLightwellPackages`, all `expand*Versions`, `filterReposByName`, `paginatePackages`, `paginateVersions`, the release-info map helpers, and the sort helpers (`sortLightwellPackages`, `lessByGroupName`, `sortLightwellVersions`) if no longer referenced. Keep `parseLightwellPackageFilters`, `parseLightwellPackageVersionFilters`, `validateContentType`, `validContentTypes`, `parseSortBy` **only if still used** (drop `parseSortBy`/sort helpers since sorting is now in SQL). Remove the `TangClient`/`PulpClient` struct fields and constructor args **only if** no other method in this file uses them; the `RegisterLightwellPackageRoutes` signature is called from `pkg/handler/api.go:126` and `pkg/handler/lightwell/packages.go` — if you change its signature, update those callers. Simplest low-risk option: keep the struct fields and constructor signature unchanged (leave `TangClient`/`PulpClient` set but unused) to avoid touching callers; remove them only if lint flags unused struct fields (it will not — they are struct fields, not locals).

> `FeatureServiceEntitledFeatures` in Steps 3-4 is a placeholder for however the handler should obtain entitled features. Determine the real accessor: the feature-service client is reachable via the same path `RepositoryConfig.List` uses internally (`r.fsClient.GetEntitledFeatures`). If there is no handler-level accessor, add a thin `DaoRegistry` method or inject the `feature_service_client.FeatureServiceClient` into `LightwellPackagesHandler` (it is constructed in `RegisterRoutes`). Use the existing pattern other handlers use to call `GetEntitledFeatures`; do not invent a new client. This is the one spot to verify against the actual handler wiring during implementation.

Run: `goimports -w pkg/handler/lightwell_packages.go`

- [ ] **Step 6: Run handler tests**

Run: `go test ./pkg/handler/ -run Lightwell -v`
Expected: PASS.

- [ ] **Step 7: Verify no OpenAPI drift**

Run: `git diff --exit-code api/openapi.json`
Expected: no diff. If swag annotations were unchanged this passes; if it fails, run `make openapi-doc` and inspect — response types did not change, so investigate any diff before committing.

- [ ] **Step 8: Commit**

```bash
git add pkg/handler/lightwell_packages.go pkg/handler/lightwell_packages_test.go
git commit -m "feat: serve lightwell packages endpoints from the DB mirror"
```

---

### Task 9: Full build, lint, and integration verification

**Files:** none (verification only), plus any small fixes surfaced.

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: success.

- [ ] **Step 2: Run the full affected test packages**

Run: `go test ./pkg/models/... ./pkg/lightwell/... ./pkg/dao/... ./pkg/clients/pulp_client/... ./pkg/external_repos/commands/... ./pkg/handler/...`
Expected: PASS.

- [ ] **Step 3: Lint**

Run: `golangci-lint run --timeout=5m`
If issues: `golangci-lint run --fix` then re-run until `0 issues`.

- [ ] **Step 4: Confirm generated-file and migration guards**

Run: `git diff --exit-code api/openapi.json` (no drift) and confirm `db/migrations.latest` matches the newest migration timestamp (the app's `checkLatestMigrationFile` runs on migrate).

- [ ] **Step 5: Manual smoke of the import (optional, if a dev Pulp/DB is available)**

Run: `go run ./cmd/external-repos import-lightwell-packages --force`
Expected: logs "Successfully imported lightwell packages"; `lightwell_packages`/`lightwell_package_versions` populated for Lightwell repos; a subsequent run without `--force` reports no changes (skips unchanged repos).

- [ ] **Step 6: Final commit if fixes were needed**

```bash
git add -A
git commit -m "chore: lint and verification fixes for lightwell package mirror"
```
