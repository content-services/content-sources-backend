package pulp_client

import zest "github.com/content-services/zest/release/v3"

// GetRpmRepositoryVersion Finds a repository version given an  href
func (r *pulpDaoImpl) GetRpmRepositoryVersion(href string) (*zest.RepositoryVersionResponse, error) {
	resp, httpResp, err := r.client.RepositoriesRpmVersionsApi.RepositoriesRpmRpmVersionsRead(r.ctx, href).Execute()

	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	return resp, nil
}
