package pulp_client

import zest "github.com/content-services/zest/release/v2024"

// CreateRpmDistribution Creates a Distribution
func (r *pulpDaoImpl) CreateRpmDistribution(publicationHref string, name string, basePath string, contentGuardHref *string) (*string, error) {
	dist := *zest.NewRpmRpmDistribution(basePath, name)
	if contentGuardHref != nil {
		dist.SetContentGuard(*contentGuardHref)
	}

	dist.SetPublication(publicationHref)
	resp, httpResp, err := r.client.DistributionsRpmAPI.DistributionsRpmRpmCreate(r.ctx, r.domainName).RpmRpmDistribution(dist).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error creating rpm distributions", httpResp, err)
	}

	taskHref := resp.GetTask()
	return &taskHref, nil
}

func (r *pulpDaoImpl) FindDistributionByPath(path string) (*zest.RpmRpmDistributionResponse, error) {
	resp, httpResp, err := r.client.DistributionsRpmAPI.DistributionsRpmRpmList(r.ctx, r.domainName).BasePath(path).Execute()
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

func (r *pulpDaoImpl) DeleteRpmDistribution(rpmDistributionHref string) (string, error) {
	resp, httpResp, err := r.client.DistributionsRpmAPI.DistributionsRpmRpmDelete(r.ctx, rpmDistributionHref).Execute()
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

func (r *pulpDaoImpl) UpdateRpmDistribution(rpmDistributionHref string, rpmPublicationHref string, distributionName string, basePath string) (string, error) {
	patchedRpmDistribution := zest.PatchedrpmRpmDistribution{}

	patchedRpmDistribution.Name = &distributionName
	patchedRpmDistribution.BasePath = &basePath

	var pub zest.NullableString
	pub.Set(&rpmPublicationHref)
	patchedRpmDistribution.SetPublication(rpmPublicationHref)

	resp, httpResp, err := r.client.DistributionsRpmAPI.DistributionsRpmRpmPartialUpdate(r.ctx, rpmDistributionHref).PatchedrpmRpmDistribution(patchedRpmDistribution).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error listing rpm distributions", httpResp, err)
	}
	defer httpResp.Body.Close()

	return resp.Task, nil
}
