package pulp_client

import zest "github.com/content-services/zest/release/v3"

// CreateRpmDistribution Creates a Distribution
func (r *pulpDaoImpl) CreateRpmDistribution(publicationHref string, name string, basePath string) (*string, error) {
	dist := *zest.NewRpmRpmDistribution(basePath, name)
	dist.SetPublication(publicationHref)
	resp, httpResp, err := r.client.DistributionsRpmApi.DistributionsRpmRpmCreate(r.ctx).RpmRpmDistribution(dist).Execute()
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	taskHref := resp.GetTask()
	return &taskHref, nil
}

func (r *pulpDaoImpl) FindDistributionByPath(path string) (*zest.RpmRpmDistributionResponse, error) {
	resp, httpResp, err := r.client.DistributionsRpmApi.DistributionsRpmRpmList(r.ctx).BasePath(path).Execute()
	if err != nil {
		return nil, err
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
	resp, httpResp, err := r.client.DistributionsRpmApi.DistributionsRpmRpmDelete(r.ctx, rpmDistributionHref).Execute()
	if err != nil {
		if err.Error() == "404 Not Found" {
			return "", nil
		}
		return "", err
	}
	defer httpResp.Body.Close()
	return resp.Task, nil
}
