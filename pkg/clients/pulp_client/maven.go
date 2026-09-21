package pulp_client

import (
	"context"
	"fmt"
	"strings"

	zest "github.com/content-services/zest/release/v2026"
)

const mavenPackageContentPageSize int32 = 300

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
