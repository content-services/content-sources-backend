package pulp_client

import zest "github.com/content-services/zest/release/v3"

// CreateRpmDistribution Creates a Distribution
func (r pulpDaoImpl) CreateRpmDistribution(publicationHref string, name string, basePath string) (*string, error) {
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
