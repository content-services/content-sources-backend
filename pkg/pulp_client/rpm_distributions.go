package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2024"
)

// CreateRpmDistribution Creates a Distribution
func (r *pulpDaoImpl) CreateRpmDistribution(ctx context.Context, publicationHref string, name string, basePath string, contentGuardHref *string) (*string, error) {
	ctx, client := getZestClient(ctx)
	dist := *zest.NewRpmRpmDistribution(basePath, name)
	if contentGuardHref != nil {
		dist.SetContentGuard(*contentGuardHref)
	}

	dist.SetPublication(publicationHref)
	resp, httpResp, err := client.DistributionsRpmAPI.DistributionsRpmRpmCreate(ctx, r.domainName).RpmRpmDistribution(dist).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error creating rpm distributions", httpResp, err)
	}

	taskHref := resp.GetTask()
	return &taskHref, nil
}

func (r *pulpDaoImpl) FindDistributionByPath(ctx context.Context, path string) (*zest.RpmRpmDistributionResponse, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.DistributionsRpmAPI.DistributionsRpmRpmList(ctx, r.domainName).BasePath(path).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing rpm distributions", httpResp, err)
	}
	defer httpResp.Body.Close()
	res := resp.GetResults()
	if len(res) > 0 {
		return &res[0], nil
	} else {
		return nil, nil
	}
}

func (r *pulpDaoImpl) DeleteRpmDistribution(ctx context.Context, rpmDistributionHref string) (string, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.DistributionsRpmAPI.DistributionsRpmRpmDelete(ctx, rpmDistributionHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if err.Error() == "404 Not Found" {
			return "", nil
		}
		return "", errorWithResponseBody("error deleting rpm distribution", httpResp, err)
	}
	defer httpResp.Body.Close()
	return resp.Task, nil
}
