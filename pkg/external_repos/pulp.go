package external_repos

import (
	"fmt"
	"os"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
)

// CreatePulpRepoFromURL
func CreatePulpRepoFromURL(orgId string, url string) []error {
	var errs []error
	daoReg := dao.GetDaoRegistry(db.DB)

	response, _, err := daoReg.RepositoryConfig.List(orgId, api.PaginationData{Limit: 1}, api.FilterData{URL: url})
	if err != nil || len(response.Data) == 0 {
		fmt.Fprintf(os.Stderr, "\n\nDidn't find URL reference in repoConfig: %v\n\n", url)
		return []error{err}
	}

	pulpClient := pulp_client.GetPulpClient()

	err = tasks.SnapshotRepository(tasks.SnapshotOptions{
		OrgId:          orgId,
		RepoConfigUuid: response.Data[0].UUID,
		DaoRegistry:    daoReg,
		PulpClient:     pulpClient,
	})

	if err != nil {
		errs = append(errs, fmt.Errorf("Error creating pulp reference for %s: %s", url, err.Error()))
	}

	return errs
}
