package pulp_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	zest "github.com/content-services/zest/release/v2026"
)

const mavenPackageContentPageSize int32 = 300

// ListMavenPackages lists distinct (group_id, artifact_id) rows for a Maven repository.
//
// Zest's generated Packages() builder exposes search and ordering but not limit/offset.
// Pulp accepts all four query params on GET {repoHref}packages/, so this thin GET uses the
// same HTTP client, auth, and base URL as getZestClient until zest generates Limit/Offset.
// TODO: Switch to Zest's generated Packages() builder when
// https://github.com/pulp/pulp_maven/issues/481 is merged and new Zest version is published.
func (r *pulpDaoImpl) ListMavenPackages(ctx context.Context, repoHref string, search string, limit, offset int) (zest.PaginatedMavenRepositoryPackageListResponse, error) {
	var empty zest.PaginatedMavenRepositoryPackageListResponse
	if repoHref == "" {
		return empty, fmt.Errorf("maven repository href is required")
	}

	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return empty, err
	}

	cfg := client.GetConfig()
	serverURL, err := cfg.ServerURLWithContext(ctx, "RepositoriesMavenAPIService.RepositoriesMavenMavenPackages")
	if err != nil {
		return empty, err
	}

	reqURL, err := mavenRepoResourceURL(serverURL, repoHref, "packages/")
	if err != nil {
		return empty, err
	}

	httpResp, err := zestGET(ctx, client, reqURL, mavenPackagesQuery(search, limit, offset))
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return empty, err
	}
	if httpResp.StatusCode >= 300 {
		return empty, errorWithResponseBody("error listing maven packages", httpResp, fmt.Errorf("unexpected http status %s", httpResp.Status))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return empty, fmt.Errorf("error reading maven packages response: %w", err)
	}

	var resp zest.PaginatedMavenRepositoryPackageListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return empty, fmt.Errorf("error decoding maven packages response: %w", err)
	}
	if resp.Results == nil {
		resp.Results = []zest.MavenRepositoryPackageResponse{}
	}
	return resp, nil
}

func mavenPackagesQuery(search string, limit, offset int) url.Values {
	q := url.Values{}
	q.Set("search", search)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("ordering", "group_id,artifact_id")
	return q
}

func mavenRepoResourceURL(serverURL, repoHref, resource string) (string, error) {
	path := strings.TrimRight(serverURL, "/") + "/{maven_maven_repository_href}" + resource
	path = strings.Replace(path, "{maven_maven_repository_href}", url.PathEscape(repoHref), 1)
	return url.PathUnescape(path)
}

func zestGET(ctx context.Context, client *zest.APIClient, rawURL string, query url.Values) (*http.Response, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	q := parsed.Query()
	for key, vals := range query {
		for _, val := range vals {
			q.Add(key, val)
		}
	}
	parsed.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	if auth, ok := ctx.Value(zest.ContextBasicAuth).(zest.BasicAuth); ok {
		req.SetBasicAuth(auth.UserName, auth.Password)
	}

	cfg := client.GetConfig()
	for header, value := range cfg.DefaultHeader {
		req.Header.Add(header, value)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return httpClient.Do(req)
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

	resp, httpResp, err := client.RepositoriesMavenAPI.RepositoriesMavenMavenMetrics(ctx, repoHref).Execute()
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

	resp, httpResp, err := client.RepositoriesMavenAPI.RepositoriesMavenMavenRead(ctx, repoHref).Execute()
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
