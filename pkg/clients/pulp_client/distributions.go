package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2026"
)

func (r *pulpDaoImpl) FindGenericDistributionByBasePath(ctx context.Context, basePath string) (*zest.DistributionResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, httpResp, err := client.DistributionsAPI.DistributionsList(ctx, r.domainName).BasePath(basePath).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error finding distribution by base path", httpResp, err)
	}
	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	}
	return nil, nil
}
