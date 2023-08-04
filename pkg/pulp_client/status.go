package pulp_client

import zest "github.com/content-services/zest/release/v2023"

func (r *pulpDaoImpl) Status() (*zest.StatusResponse, error) {
	// Change this back to StatusRead(r.ctx) on next zest update
	status, resp, err := r.client.StatusAPI.StatusRead1(r.ctx).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return status, nil
}
