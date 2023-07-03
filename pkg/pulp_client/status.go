package pulp_client

import zest "github.com/content-services/zest/release/v3"

// Creates a remote
func (r *pulpDaoImpl) Status() (*zest.StatusResponse, error) {
	status, resp, err := r.client.StatusApi.StatusRead(r.ctx).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return status, nil
}
