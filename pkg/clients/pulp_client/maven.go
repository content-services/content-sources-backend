package pulp_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2026"
)

const mavenPackageContentPageSize int32 = 300

// mavenFlatCursorPagination selects Pulp's count-free cursor pages.
// The flat catalog ignores limit/offset in this mode and omits count.
const mavenFlatCursorPagination = "cursor"

// zestHrefPathParam strips a leading slash so zest does not request
// {server}//api/pulp/... (Pulp hrefs are already absolute paths).
func zestHrefPathParam(href string) string {
	return strings.TrimLeft(href, "/")
}

// ListMavenPackages lists distinct (group_id, artifact_id) rows for a Maven repository.
func (r *pulpDaoImpl) ListMavenPackages(ctx context.Context, repoHref string, search string, limit, offset int) (zest.PaginatedMavenRepositoryPackageListResponse, error) {
	var empty zest.PaginatedMavenRepositoryPackageListResponse
	if repoHref == "" {
		return empty, fmt.Errorf("maven repository href is required")
	}

	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return empty, err
	}

	resp, httpResp, err := client.RepositoriesMavenAPI.RepositoriesMavenMavenPackages(ctx, zestHrefPathParam(repoHref)).
		Search(search).
		Limit(int32(limit)).   //nolint:gosec // G115: pagination / catalog page sizes are well below int32 max
		Offset(int32(offset)). //nolint:gosec // G115: pagination / catalog page sizes are well below int32 max
		Ordering([]string{"group_id", "artifact_id"}).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return empty, errorWithResponseBody("error listing maven packages", httpResp, err)
	}
	if resp == nil {
		return empty, fmt.Errorf("empty maven packages response")
	}
	if resp.Results == nil {
		resp.Results = []zest.MavenRepositoryPackageResponse{}
	}
	return *resp, nil
}

// ListMavenFlatPackages lists one row per stored Maven version from packages/flat/.
// The first request sets pagination=cursor. Later pages follow next, because cursor
// mode ignores offset and does not return a total count.
func (r *pulpDaoImpl) ListMavenFlatPackages(ctx context.Context, repoHref string) (zest.PaginatedMavenRepositoryFlatPackageResponseList, error) {
	var empty zest.PaginatedMavenRepositoryFlatPackageResponseList
	if repoHref == "" {
		return empty, fmt.Errorf("maven repository href is required")
	}

	var all []zest.MavenRepositoryFlatPackageResponse
	var pageURL string
	seen := map[string]struct{}{}
	for {
		page, err := r.listMavenFlatPackagesPage(ctx, repoHref, pageURL, seen)
		if err != nil {
			return empty, err
		}
		all = append(all, page.Results...)
		next := page.GetNext()
		if next == "" || len(page.Results) == 0 {
			break
		}
		pageURL, err = resolvePulpPageURL(config.Get().Clients.Pulp.Server, next)
		if err != nil {
			return empty, err
		}
	}
	if all == nil {
		all = []zest.MavenRepositoryFlatPackageResponse{}
	}
	if len(all) > math.MaxInt32 {
		return empty, fmt.Errorf("maven flat packages response exceeds supported count")
	}
	return zest.PaginatedMavenRepositoryFlatPackageResponseList{Count: int32(len(all)), Results: all}, nil //nolint:gosec // G115: bounded by the check above
}

func (r *pulpDaoImpl) listMavenFlatPackagesPage(ctx context.Context, repoHref, pageURL string, seen map[string]struct{}) (zest.PaginatedMavenRepositoryFlatPackageResponseList, error) {
	var empty zest.PaginatedMavenRepositoryFlatPackageResponseList
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return empty, err
	}
	if pageURL == "" {
		_, httpResp, callErr := client.RepositoriesMavenAPI.RepositoriesMavenMavenPackagesFlat(ctx, zestHrefPathParam(repoHref)).
			Pagination(mavenFlatCursorPagination).
			Execute()
		if httpResp != nil {
			defer httpResp.Body.Close()
		}
		return decodeMavenFlatPackagePage(httpResp, callErr)
	}
	if _, dup := seen[pageURL]; dup {
		return empty, fmt.Errorf("maven flat packages cursor page repeated")
	}
	seen[pageURL] = struct{}{}

	httpClient := client.GetConfig().HTTPClient
	if httpClient == nil {
		return empty, fmt.Errorf("pulp http client is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return empty, err
	}
	req.Header.Set("Accept", "application/json")
	if id := client.GetConfig().DefaultHeader["Correlation-ID"]; id != "" {
		req.Header.Set("Correlation-ID", id)
	}
	if ua := client.GetConfig().UserAgent; ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if auth, ok := ctx.Value(zest.ContextBasicAuth).(zest.BasicAuth); ok {
		req.SetBasicAuth(auth.UserName, auth.Password)
	}
	httpResp, err := httpClient.Do(req)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return empty, fmt.Errorf("error listing maven flat packages: %w", err)
	}
	return decodeMavenFlatPackagePage(httpResp, nil)
}

// decodeMavenFlatPackagePage reads a flat catalog page.
// Cursor responses omit count, which the generated zest model requires, so this
// decodes the body directly and keeps next for the following page.
func decodeMavenFlatPackagePage(httpResp *http.Response, callErr error) (zest.PaginatedMavenRepositoryFlatPackageResponseList, error) {
	var empty zest.PaginatedMavenRepositoryFlatPackageResponseList
	if httpResp == nil || httpResp.StatusCode >= http.StatusMultipleChoices {
		if callErr == nil {
			callErr = fmt.Errorf("unexpected maven flat packages response")
		}
		return empty, errorWithResponseBody("error listing maven flat packages", httpResp, callErr)
	}
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return empty, fmt.Errorf("error reading maven flat packages response: %w", err)
	}
	page, err := unmarshalMavenFlatPackagePage(body)
	if err != nil {
		return empty, fmt.Errorf("error listing maven flat packages: %w: %s", err, string(body))
	}
	return page, nil
}

type mavenFlatPackagePage struct {
	Next    *string                                   `json:"next"`
	Results []zest.MavenRepositoryFlatPackageResponse `json:"results"`
}

func unmarshalMavenFlatPackagePage(body []byte) (zest.PaginatedMavenRepositoryFlatPackageResponseList, error) {
	var empty zest.PaginatedMavenRepositoryFlatPackageResponseList
	var raw mavenFlatPackagePage
	if err := json.Unmarshal(body, &raw); err != nil {
		return empty, err
	}
	if raw.Results == nil {
		raw.Results = []zest.MavenRepositoryFlatPackageResponse{}
	}
	page := zest.PaginatedMavenRepositoryFlatPackageResponseList{Results: raw.Results}
	if raw.Next != nil && *raw.Next != "" {
		page.SetNext(*raw.Next)
	}
	return page, nil
}

// resolvePulpPageURL keeps the page path and query from next, and requests the configured Pulp server.
func resolvePulpPageURL(server, next string) (string, error) {
	nextURL, err := url.Parse(next)
	if err != nil || next == "" || (nextURL.Path == "" && nextURL.RawQuery == "" && nextURL.Host == "") {
		return "", fmt.Errorf("invalid maven flat packages page url %q", next)
	}
	base, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid pulp server url %q", server)
	}
	if nextURL.Host == "" {
		return base.ResolveReference(nextURL).String(), nil
	}
	rewritten := &url.URL{
		Scheme:   base.Scheme,
		Host:     base.Host,
		Path:     nextURL.Path,
		RawQuery: nextURL.RawQuery,
	}
	return rewritten.String(), nil
}

// GetMavenRepositoryMetrics returns distinct MavenPackage GA / GAV / base-version counts for a repo.
func (r *pulpDaoImpl) GetMavenRepositoryMetrics(ctx context.Context, repoHref string) (zest.MavenRepositoryMetricsResponse, error) {
	var empty zest.MavenRepositoryMetricsResponse
	if repoHref == "" {
		return empty, fmt.Errorf("maven repository href is required")
	}

	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return empty, err
	}

	resp, httpResp, err := client.RepositoriesMavenAPI.RepositoriesMavenMavenMetrics(ctx, zestHrefPathParam(repoHref)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return empty, errorWithResponseBody("error reading maven repository metrics", httpResp, err)
	}
	if resp == nil {
		return empty, fmt.Errorf("empty maven repository metrics response")
	}
	return *resp, nil
}

// ListMavenPackageContent lists MavenPackage content units for a GA, optionally filtered by base_version.
// CollapseBuilds is never set: callers that need nested builds[] must see every rebuild.
// VersionStartswith is never used; PackageGet must pass base_version= for an exact logical version.
func (r *pulpDaoImpl) ListMavenPackageContent(ctx context.Context, repoHref, groupId, artifactId, baseVersion string) ([]zest.MavenMavenPackageResponse, error) {
	versionHref, err := r.mavenLatestVersionHref(ctx, repoHref)
	if err != nil {
		return nil, err
	}

	var all []zest.MavenMavenPackageResponse
	var offset int32
	for {
		page, total, err := r.listMavenPackageContentPage(ctx, groupId, artifactId, baseVersion, versionHref, offset, mavenPackageContentPageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) == 0 || int64(len(all)) >= int64(total) {
			break
		}
		offset += mavenPackageContentPageSize
	}
	if all == nil {
		all = []zest.MavenMavenPackageResponse{}
	}
	return all, nil
}

func (r *pulpDaoImpl) mavenLatestVersionHref(ctx context.Context, repoHref string) (string, error) {
	if repoHref == "" {
		return "", fmt.Errorf("maven repository href is required")
	}

	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

	resp, httpResp, err := client.RepositoriesMavenAPI.RepositoriesMavenMavenRead(ctx, zestHrefPathParam(repoHref)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error reading maven repository", httpResp, err)
	}
	if resp == nil {
		return "", fmt.Errorf("empty maven repository response")
	}

	href := resp.GetLatestVersionHref()
	if href == "" {
		return "", fmt.Errorf("maven repository %s has no latest version", repoHref)
	}
	return href, nil
}

func (r *pulpDaoImpl) listMavenPackageContentPage(ctx context.Context, groupId, artifactId, baseVersion, repositoryVersion string, offset, limit int32) ([]zest.MavenMavenPackageResponse, int32, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, 0, err
	}

	req := client.ContentPackageAPI.ContentMavenPackageList(ctx, r.domainName).
		GroupId(groupId).
		ArtifactId(artifactId).
		RepositoryVersion(repositoryVersion).
		Limit(limit).
		Offset(offset).
		Ordering([]string{"-pulp_created"})
	if baseVersion != "" {
		req = req.BaseVersion(baseVersion)
	}

	resp, httpResp, err := req.Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, 0, errorWithResponseBody("error listing maven package content", httpResp, err)
	}
	if resp == nil {
		return []zest.MavenMavenPackageResponse{}, 0, nil
	}
	if resp.Results == nil {
		resp.Results = []zest.MavenMavenPackageResponse{}
	}
	return resp.Results, resp.Count, nil
}
