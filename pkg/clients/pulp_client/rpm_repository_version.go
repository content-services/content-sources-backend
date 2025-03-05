package pulp_client

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2024"
)

// GetRpmRepositoryVersion Finds a repository version given its href
func (r *pulpDaoImpl) GetRpmRepositoryVersion(ctx context.Context, href string) (*zest.RepositoryVersionResponse, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmVersionsAPI.RepositoriesRpmRpmVersionsRead(ctx, href).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error reading rpm repository versions", httpResp, err)
	}

	return resp, nil
}

// DeleteRpmRepositoryVersion starts task to delete repository version and returns delete task's href
func (r *pulpDaoImpl) DeleteRpmRepositoryVersion(ctx context.Context, href string) (*string, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmVersionsAPI.RepositoriesRpmRpmVersionsDelete(ctx, href).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if err.Error() == "404 Not Found" {
			return nil, nil
		}
		return nil, errorWithResponseBody("error deleting rpm repository versions", httpResp, err)
	}
	return &resp.Task, nil
}

func (r *pulpDaoImpl) RepairRpmRepositoryVersion(ctx context.Context, href string) (string, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmVersionsAPI.RepositoriesRpmRpmVersionsRepair(ctx, href).
		Repair(zest.Repair{VerifyChecksums: utils.Ptr(true)}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error repairing rpm repository versions", httpResp, err)
	}
	return resp.Task, nil
}

func (r *pulpDaoImpl) ModifyRpmRepositoryContent(ctx context.Context, repoHref string, contentHrefsToAdd []string, contentHrefsToRemove []string) (string, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmAPI.RepositoriesRpmRpmModify(ctx, repoHref).RepositoryAddRemoveContent(zest.RepositoryAddRemoveContent{
		AddContentUnits:    contentHrefsToAdd,
		RemoveContentUnits: contentHrefsToRemove,
	}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error modifying rpm repository content", httpResp, err)
	}
	return resp.Task, nil
}
