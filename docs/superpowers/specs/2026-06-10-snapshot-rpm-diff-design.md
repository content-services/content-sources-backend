# Snapshot RPM Diff API

## Problem

Users need to see which RPM packages were added and removed between consecutive repository snapshots. The frontend will render this as a diff view (green for added, red for removed), with updated packages (same name, different version) grouped together.

## Data Source

Pulp's `ContentRpmPackagesList` API supports filtering by `repository_version_added` and `repository_version_removed`. These filters return the individual RPM packages that were added or removed in a specific repository version (relative to its predecessor). This is the same data that Pulp's `ContentSummary` counts reference, but at the individual package level.

## API Endpoint

**Route:** `GET /snapshots/:snapshot_uuid/rpms/diff`

**Query parameters:**

| Parameter | Type   | Default | Description |
|-----------|--------|---------|-------------|
| `offset`  | int    | 0       | Pagination offset |
| `limit`   | int    | 100     | Pagination limit |
| `search`  | string | (none)  | Filter by package name |

**Response:**

```json
{
  "data": [
    {
      "name": "bash",
      "arch": "x86_64",
      "version": "5.2.15",
      "release": "3.el9",
      "epoch": "0",
      "summary": "The GNU Bourne Again shell",
      "status": "removed"
    },
    {
      "name": "bash",
      "arch": "x86_64",
      "version": "5.2.26",
      "release": "4.el9",
      "epoch": "0",
      "summary": "The GNU Bourne Again shell",
      "status": "added"
    },
    {
      "name": "curl",
      "arch": "x86_64",
      "version": "8.5.0",
      "release": "1.el9",
      "epoch": "0",
      "summary": "A utility for getting files from remote servers",
      "status": "added"
    }
  ],
  "meta": { "limit": 100, "offset": 0, "count": 3 },
  "links": { "first": "...", "last": "..." }
}
```

**Sorting/grouping:**
- Primary sort: package name (alphabetical ascending)
- Within same name: `"removed"` before `"added"` (old version appears above new version)

## New API Types

- `SnapshotRpmDiff` — extends `SnapshotRpm` fields with `Status string` (`"added"` or `"removed"`)
- `SnapshotRpmDiffCollectionResponse` — wraps `[]SnapshotRpmDiff` with standard `Meta` and `Links`

## Pulp Client Layer

Two new methods on the `PulpClient` interface, following the existing `ListVersionPackages` pattern:

```go
ListVersionAddedPackages(ctx context.Context, versionHref string, offset, limit int32) ([]zest.RpmPackageResponse, int, error)
ListVersionRemovedPackages(ctx context.Context, versionHref string, offset, limit int32) ([]zest.RpmPackageResponse, int, error)
```

These call `ContentRpmPackagesList` with `.RepositoryVersionAdded(versionHref)` and `.RepositoryVersionRemoved(versionHref)` respectively, using the same `RpmFields` field filter as the existing package listing.

## Handler Data Flow

1. Handler receives request, extracts `snapshot_uuid` from path
2. Looks up snapshot from DB via `Snapshot.FetchModel()` to get `VersionHref`
3. Resolves Pulp domain via `Domain.Fetch(orgID)`
4. Fetches **all** added packages from Pulp (paginating through Pulp's results internally)
5. Fetches **all** removed packages from Pulp (same)
6. Optionally filters both lists by `search` query param (name substring match)
7. Merges into a single list, tagging each entry with `status: "added"` or `"removed"`
8. Sorts alphabetically by name, then `"removed"` before `"added"` within same name
9. Applies `offset`/`limit` pagination on the merged list
10. Returns standard collection response

**Why fetch all from Pulp and paginate in-memory:** The merged, alphabetically-grouped list cannot be produced by paginating Pulp's added and removed results separately. For typical snapshot diffs (tens to low thousands of changed packages), fetching all is practical.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/clients/pulp_client/interfaces.go` | Add `ListVersionAddedPackages` and `ListVersionRemovedPackages` to `PulpClient` interface |
| `pkg/clients/pulp_client/package.go` | Implement the two new methods |
| `pkg/clients/pulp_client/pulp_client_mock.go` | Add mock implementations |
| `pkg/api/rpms.go` | Add `SnapshotRpmDiff` and `SnapshotRpmDiffCollectionResponse` types |
| `pkg/handler/rpms.go` | Add `snapshotRpmDiff` handler, register route in `RegisterRpmRoutes` |
| `pkg/handler/rpms_test.go` | Tests for the new handler |

## What Is NOT in Scope

- Comparing arbitrary snapshot pairs (only consecutive snapshots via Pulp's built-in diff)
- Content types other than RPM packages (no advisories, modules, etc.)
- Database migrations or new models
- Frontend implementation (grouping updated packages by name is a frontend concern)
