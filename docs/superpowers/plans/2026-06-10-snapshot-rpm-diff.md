# Snapshot RPM Diff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `GET /snapshots/:snapshot_uuid/rpms/diff` endpoint that returns the individual RPM packages added and removed in a snapshot, merged into a single alphabetically-sorted paginated list.

**Architecture:** The handler fetches the snapshot's `VersionHref` from the DB, then calls two new Pulp client methods (`ListVersionAllAddedPackages` / `ListVersionAllRemovedPackages`) which use Pulp's `RepositoryVersionAdded` / `RepositoryVersionRemoved` query filters on `ContentRpmPackagesList`. The handler merges both lists, tags each entry with a status, sorts by name (removed before added within the same name), optionally filters by search term, and paginates in-memory.

**Tech Stack:** Go, Echo HTTP framework, Pulp RPM API (via zest client v2026), mockery for mock generation

---

### Task 1: Add API types for snapshot RPM diff

**Files:**
- Modify: `pkg/api/rpms.go`

- [ ] **Step 1: Add `SnapshotRpmDiff` struct**

Add the following types after the existing `SnapshotRpm` struct (around line 23) in `pkg/api/rpms.go`:

```go
type SnapshotRpmDiff struct {
	Name    string `json:"name"`    // The rpm package name
	Arch    string `json:"arch"`    // The architecture of the rpm
	Version string `json:"version"` // The version of the rpm
	Release string `json:"release"` // The release of the rpm
	Epoch   string `json:"epoch"`   // The epoch of the rpm
	Summary string `json:"summary"` // The summary of the rpm
	Status  string `json:"status"`  // "added" or "removed"
}

type SnapshotRpmDiffCollectionResponse struct {
	Data  []SnapshotRpmDiff `json:"data"`  // List of rpm diffs
	Meta  ResponseMetadata  `json:"meta"`  // Metadata about the request
	Links Links             `json:"links"` // Links to other pages of results
}

func (r *SnapshotRpmDiffCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./pkg/api/...`
Expected: clean compile, no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/api/rpms.go
git commit -m "feat: add SnapshotRpmDiff API types for snapshot diff endpoint"
```

---

### Task 2: Add Pulp client methods for fetching added/removed packages

**Files:**
- Modify: `pkg/clients/pulp_client/interfaces.go`
- Modify: `pkg/clients/pulp_client/package.go`

- [ ] **Step 1: Add methods to the PulpClient interface**

In `pkg/clients/pulp_client/interfaces.go`, add the following two methods under the `// Package` section (after line 52, after `ListVersionAllPackages`):

```go
	ListVersionAllAddedPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error)
	ListVersionAllRemovedPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error)
```

- [ ] **Step 2: Implement the helper methods in package.go**

Add the following functions at the end of `pkg/clients/pulp_client/package.go`:

```go
func (r *pulpDaoImpl) ListVersionAddedPackages(ctx context.Context, versionHref string, offset, limit int32) (pkgs []zest.RpmPackageResponse, total int, err error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return pkgs, 0, err
	}
	resp, httpResp, err := client.ContentPackagesAPI.ContentRpmPackagesList(ctx, r.domainName).RepositoryVersionAdded(versionHref).Limit(limit).Fields(RpmFields).Offset(offset).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return pkgs, 0, errorWithResponseBody("error listing added packages for version", httpResp, err)
	}
	return resp.Results, int(resp.Count), err
}

func (r *pulpDaoImpl) ListVersionAllAddedPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error) {
	initial := int32(0)
	limit := int32(300)
	pkgs, total, err := r.ListVersionAddedPackages(ctx, versionHref, initial, limit)
	if err != nil {
		return nil, err
	}
	for len(pkgs) < total {
		initial += limit
		pkgList, _, err := r.ListVersionAddedPackages(ctx, versionHref, initial, limit)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkgList...)
	}
	return pkgs, nil
}

func (r *pulpDaoImpl) ListVersionRemovedPackages(ctx context.Context, versionHref string, offset, limit int32) (pkgs []zest.RpmPackageResponse, total int, err error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return pkgs, 0, err
	}
	resp, httpResp, err := client.ContentPackagesAPI.ContentRpmPackagesList(ctx, r.domainName).RepositoryVersionRemoved(versionHref).Limit(limit).Fields(RpmFields).Offset(offset).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return pkgs, 0, errorWithResponseBody("error listing removed packages for version", httpResp, err)
	}
	return resp.Results, int(resp.Count), err
}

func (r *pulpDaoImpl) ListVersionAllRemovedPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error) {
	initial := int32(0)
	limit := int32(300)
	pkgs, total, err := r.ListVersionRemovedPackages(ctx, versionHref, initial, limit)
	if err != nil {
		return nil, err
	}
	for len(pkgs) < total {
		initial += limit
		pkgList, _, err := r.ListVersionRemovedPackages(ctx, versionHref, initial, limit)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkgList...)
	}
	return pkgs, nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./pkg/clients/pulp_client/...`
Expected: clean compile, no errors

- [ ] **Step 4: Regenerate mocks**

Run: `make mock`
Expected: `pkg/clients/pulp_client/pulp_client_mock.go` is updated with new mock methods for `ListVersionAllAddedPackages` and `ListVersionAllRemovedPackages`.

- [ ] **Step 5: Verify mocks compile**

Run: `go build ./pkg/clients/pulp_client/...`
Expected: clean compile

- [ ] **Step 6: Commit**

```bash
git add pkg/clients/pulp_client/interfaces.go pkg/clients/pulp_client/package.go pkg/clients/pulp_client/pulp_client_mock.go
git commit -m "feat: add Pulp client methods for fetching added/removed packages by version"
```

---

### Task 3: Add the snapshot RPM diff handler and route

**Files:**
- Modify: `pkg/handler/rpms.go`
- Modify: `pkg/handler/rpms_test.go`

- [ ] **Step 1: Write the `mergeRpmDiffs` unit test first**

The `mergeRpmDiffs` function is the core custom logic (merge, sort, search filter). Test it directly without needing HTTP or Pulp mocking. Add the following test to `pkg/handler/rpms_test.go`:

```go
func (suite *RpmSuite) TestMergeRpmDiffs() {
	t := suite.T()

	curlAdded := zest.RpmPackageResponse{}
	curlAdded.SetName("curl")
	curlAdded.SetArch("x86_64")
	curlAdded.SetVersion("8.5.0")
	curlAdded.SetRelease("1.el9")
	curlAdded.SetEpoch("0")
	curlAdded.SetSummary("A utility for getting files from remote servers")

	bashAdded := zest.RpmPackageResponse{}
	bashAdded.SetName("bash")
	bashAdded.SetArch("x86_64")
	bashAdded.SetVersion("5.2.26")
	bashAdded.SetRelease("4.el9")
	bashAdded.SetEpoch("0")
	bashAdded.SetSummary("The GNU Bourne Again shell")

	bashRemoved := zest.RpmPackageResponse{}
	bashRemoved.SetName("bash")
	bashRemoved.SetArch("x86_64")
	bashRemoved.SetVersion("5.2.15")
	bashRemoved.SetRelease("3.el9")
	bashRemoved.SetEpoch("0")
	bashRemoved.SetSummary("The GNU Bourne Again shell")

	merged := mergeRpmDiffs(
		[]zest.RpmPackageResponse{curlAdded, bashAdded},
		[]zest.RpmPackageResponse{bashRemoved},
		"",
	)

	require.Len(t, merged, 3)
	// bash removed (old) comes first
	assert.Equal(t, "bash", merged[0].Name)
	assert.Equal(t, "removed", merged[0].Status)
	assert.Equal(t, "5.2.15", merged[0].Version)
	// bash added (new) comes second
	assert.Equal(t, "bash", merged[1].Name)
	assert.Equal(t, "added", merged[1].Status)
	assert.Equal(t, "5.2.26", merged[1].Version)
	// curl added comes last (alphabetically after bash)
	assert.Equal(t, "curl", merged[2].Name)
	assert.Equal(t, "added", merged[2].Status)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/handler/ -run TestRpmSuite/TestMergeRpmDiffs -v`
Expected: FAIL — `mergeRpmDiffs` is not defined yet.

- [ ] **Step 3: Add imports to rpms.go**

Add the following imports to `pkg/handler/rpms.go` (merge into the existing import block):

```go
	"sort"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	zest "github.com/content-services/zest/release/v2026"
```

- [ ] **Step 4: Register the new route**

In `pkg/handler/rpms.go`, inside the `RegisterRpmRoutes` function, add the following line after the existing snapshot routes (after line 28):

```go
	addRepoRoute(engine, http.MethodGet, "/snapshots/:uuid/rpms/diff", rh.listSnapshotRpmDiff, rbac.RbacVerbRead)
```

**Important:** This route MUST be registered BEFORE the existing `/snapshots/:uuid/rpms` route (line 26), because Echo matches routes greedily. If `/snapshots/:uuid/rpms` is registered first, requests to `/snapshots/:uuid/rpms/diff` could be matched by the `:uuid` capturing `uuid/rpms/diff`. Move the new route above line 26.

- [ ] **Step 5: Implement the handler**

Add the following handler method to `pkg/handler/rpms.go`:

```go
// listSnapshotRpmDiff godoc
// @Summary      List Snapshot RPM Diff
// @ID           listSnapshotRpmDiff
// @Description  List RPM packages added and removed in a repository snapshot.
// @Tags         rpms
// @Accept       json
// @Produce      json
// @Param		 uuid	path string true "Snapshot ID."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Success      200 {object} api.SnapshotRpmDiffCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/{uuid}/rpms/diff [get]
func (rh *RpmHandler) listSnapshotRpmDiff(c echo.Context) error {
	err := CheckSnapshotAccessible(c.Request().Context())
	if err != nil {
		return err
	}

	snapshotUUID := c.Param("uuid")
	_, orgID := getAccountIdOrgId(c)
	page := ParsePagination(c)
	search := c.QueryParam("search")

	snap, err := rh.Dao.Snapshot.FetchModel(c.Request().Context(), snapshotUUID, false)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching snapshot", err.Error())
	}

	domainName, err := rh.Dao.Domain.Fetch(c.Request().Context(), orgID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching domain", err.Error())
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	added, err := pulpClient.ListVersionAllAddedPackages(c.Request().Context(), snap.VersionHref)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error fetching added packages from Pulp", err.Error())
	}

	removed, err := pulpClient.ListVersionAllRemovedPackages(c.Request().Context(), snap.VersionHref)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error fetching removed packages from Pulp", err.Error())
	}

	merged := mergeRpmDiffs(added, removed, search)

	total := int64(len(merged))
	start := page.Offset
	end := page.Offset + page.Limit
	if start > int(total) {
		start = int(total)
	}
	if end > int(total) {
		end = int(total)
	}
	paged := merged[start:end]

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&api.SnapshotRpmDiffCollectionResponse{Data: paged}, c, total))
}

func mergeRpmDiffs(added, removed []zest.RpmPackageResponse, search string) []api.SnapshotRpmDiff {
	var merged []api.SnapshotRpmDiff
	searchLower := strings.ToLower(search)

	for _, pkg := range removed {
		name := pkg.GetName()
		if search != "" && !strings.Contains(strings.ToLower(name), searchLower) {
			continue
		}
		merged = append(merged, api.SnapshotRpmDiff{
			Name:    name,
			Arch:    pkg.GetArch(),
			Version: pkg.GetVersion(),
			Release: pkg.GetRelease(),
			Epoch:   pkg.GetEpoch(),
			Summary: pkg.GetSummary(),
			Status:  "removed",
		})
	}

	for _, pkg := range added {
		name := pkg.GetName()
		if search != "" && !strings.Contains(strings.ToLower(name), searchLower) {
			continue
		}
		merged = append(merged, api.SnapshotRpmDiff{
			Name:    name,
			Arch:    pkg.GetArch(),
			Version: pkg.GetVersion(),
			Release: pkg.GetRelease(),
			Epoch:   pkg.GetEpoch(),
			Summary: pkg.GetSummary(),
			Status:  "added",
		})
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Name != merged[j].Name {
			return merged[i].Name < merged[j].Name
		}
		if merged[i].Status != merged[j].Status {
			return merged[i].Status == "removed"
		}
		return merged[i].Version < merged[j].Version
	})

	return merged
}
```

- [ ] **Step 6: Verify the mergeRpmDiffs test passes**

Run: `go test ./pkg/handler/ -run TestRpmSuite/TestMergeRpmDiffs -v`
Expected: PASS — bash removed first, bash added second, curl added last.

- [ ] **Step 7: Commit**

```bash
git add pkg/handler/rpms.go pkg/handler/rpms_test.go
git commit -m "feat: add GET /snapshots/:uuid/rpms/diff endpoint"
```

---

### Task 4: Add test for search filtering on mergeRpmDiffs

**Files:**
- Modify: `pkg/handler/rpms_test.go`

- [ ] **Step 1: Write the search filter test**

Add the following test to `pkg/handler/rpms_test.go`:

```go
func (suite *RpmSuite) TestMergeRpmDiffsSearch() {
	t := suite.T()

	bashPkg := zest.RpmPackageResponse{}
	bashPkg.SetName("bash")
	bashPkg.SetArch("x86_64")
	bashPkg.SetVersion("5.2.26")
	bashPkg.SetRelease("4.el9")
	bashPkg.SetEpoch("0")
	bashPkg.SetSummary("The GNU Bourne Again shell")

	curlPkg := zest.RpmPackageResponse{}
	curlPkg.SetName("curl")
	curlPkg.SetArch("x86_64")
	curlPkg.SetVersion("8.5.0")
	curlPkg.SetRelease("1.el9")
	curlPkg.SetEpoch("0")
	curlPkg.SetSummary("A utility for getting files from remote servers")

	merged := mergeRpmDiffs([]zest.RpmPackageResponse{bashPkg, curlPkg}, []zest.RpmPackageResponse{}, "curl")

	require.Len(t, merged, 1)
	assert.Equal(t, "curl", merged[0].Name)
	assert.Equal(t, "added", merged[0].Status)
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./pkg/handler/ -run TestRpmSuite/TestMergeRpmDiffsSearch -v`
Expected: PASS — only curl is returned because the search filter excludes bash.

- [ ] **Step 3: Commit**

```bash
git add pkg/handler/rpms_test.go
git commit -m "test: add search filter test for mergeRpmDiffs"
```

---

### Task 5: Add test for empty diff

**Files:**
- Modify: `pkg/handler/rpms_test.go`

- [ ] **Step 1: Write empty diff test**

Add the following test to `pkg/handler/rpms_test.go`:

```go
func (suite *RpmSuite) TestMergeRpmDiffsEmpty() {
	merged := mergeRpmDiffs([]zest.RpmPackageResponse{}, []zest.RpmPackageResponse{}, "")
	assert.Empty(suite.T(), merged)
}
```

- [ ] **Step 2: Run all RPM handler tests**

Run: `go test ./pkg/handler/ -run TestRpmSuite -v`
Expected: All tests pass, including the existing ones.

- [ ] **Step 3: Commit**

```bash
git add pkg/handler/rpms_test.go
git commit -m "test: add empty diff test for mergeRpmDiffs"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run all tests in the repository**

Run: `go test ./...`
Expected: All tests pass. No regressions.

- [ ] **Step 2: Run linter if available**

Run: `make lint` (or `golangci-lint run` if available)
Expected: No new lint errors.

- [ ] **Step 3: Verify the full build**

Run: `go build ./cmd/...`
Expected: Clean compile.
