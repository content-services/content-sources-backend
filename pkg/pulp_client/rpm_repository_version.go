package pulp_client

import (
	zest "github.com/content-services/zest/release/v2024"
	"github.com/openlyinc/pointy"
)

// GetRpmRepositoryVersion Finds a repository version given its href
func (r *pulpDaoImpl) GetRpmRepositoryVersion(href string) (*zest.RepositoryVersionResponse, error) {
	resp, httpResp, err := r.client.RepositoriesRpmVersionsAPI.RepositoriesRpmRpmVersionsRead(r.ctx, href).Execute()
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	return resp, nil
}

// DeleteRpmRepositoryVersion starts task to delete repository version and returns delete task's href
func (r *pulpDaoImpl) DeleteRpmRepositoryVersion(href string) (string, error) {
	resp, httpResp, err := r.client.RepositoriesRpmVersionsAPI.RepositoriesRpmRpmVersionsDelete(r.ctx, href).Execute()
	if err != nil {
		if err.Error() == "404 Not Found" {
			return "", nil
		}
		return "", err
	}
	defer httpResp.Body.Close()
	return resp.Task, nil
}

func (r *pulpDaoImpl) RepairRpmRepositoryVersion(href string) (string, error) {
	resp, httpResp, err := r.client.RepositoriesRpmVersionsAPI.RepositoriesRpmRpmVersionsRepair(r.ctx, href).
		Repair(zest.Repair{VerifyChecksums: pointy.Pointer(true)}).Execute()
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()
	return resp.Task, nil
}
