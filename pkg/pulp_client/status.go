package pulp_client

import zest "github.com/content-services/zest/release/v2023"

func (r *pulpDaoImpl) Status() (*zest.StatusResponse, error) {
	status, resp, err := r.client.StatusAPI.StatusRead(r.ctx).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return status, nil
}
