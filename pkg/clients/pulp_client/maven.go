package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2026"
)

func NewMavenSimpleContentType(pulpClient *PulpClient) PulpSimpleContentType {
	return &mavenSimpleContentType{pulpClient: pulpClient}
}

type mavenSimpleContentType struct {
	pulpClient *PulpClient
}

func (m *mavenSimpleContentType) FetchLatestContentCounts(ctx context.Context, name string) (*zest.ContentSummaryResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	domainName := (*m.pulpClient).GetDomain()
	versionHref, err := m.fetchLatestVersionUrl(ctx, client, domainName, name)
	if err != nil {
		return nil, err
	}
	if versionHref == nil || *versionHref == "" {
		return nil, nil
	}
	version, err := m.fetchVersion(ctx, client, *versionHref)
	if err != nil {
		return nil, err
	}
	return version.ContentSummary, nil
}

func (m *mavenSimpleContentType) fetchLatestVersionUrl(ctx context.Context, zestClient *zest.APIClient, domainName string, repoName string) (repoVersionHref *string, err error) {
	resp, httpResp, err := zestClient.RepositoriesMavenAPI.RepositoriesMavenMavenList(ctx, domainName).Name(repoName).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing rpm repositories", httpResp, err)
	}
	defer httpResp.Body.Close()

	results := resp.GetResults()
	if len(results) > 0 {
		return results[0].LatestVersionHref, nil
	} else {
		return nil, nil
	}
}

func (m *mavenSimpleContentType) fetchVersion(ctx context.Context, zestClient *zest.APIClient, versionHref string) (response *zest.RepositoryVersionResponse, err error) {
	resp, httpResp, err := zestClient.RepositoriesMavenVersionsAPI.RepositoriesMavenMavenVersionsRead(ctx, versionHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing rpm repositories", httpResp, err)
	}
	defer httpResp.Body.Close()

	return resp, err
}
