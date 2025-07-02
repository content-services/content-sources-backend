package external_repos

import (
	"context"
	"embed"
	"encoding/json"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

const CommunityReposFile = "community_repos.json"

//go:embed "community_repos.json"

var crFS embed.FS

type CommunityRepo struct {
	Url                 string `json:"url"`
	Name                string `json:"name"`
	DistributionArch    string `json:"distribution_arch"`
	DistributionVersion string `json:"distribution_version"`
	GpgKey              string `json:"gpg_key"`
	Selector            string `json:"selector"`
}

func (cr CommunityRepo) ToRepositoryRequest() api.RepositoryRequest {
	return api.RepositoryRequest{
		Name:                 &cr.Name,
		URL:                  &cr.Url,
		DistributionVersions: &[]string{cr.DistributionVersion},
		DistributionArch:     &cr.DistributionArch,
		GpgKey:               &cr.GpgKey,
		MetadataVerification: utils.Ptr(false),
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginCommunity),
		ContentType:          utils.Ptr(config.ContentTypeRpm),
	}
}

type CommunityRepoImporter struct {
	daoReg *dao.DaoRegistry
}

func NewCommunityRepos(daoReg *dao.DaoRegistry) CommunityRepoImporter {
	return CommunityRepoImporter{
		daoReg: daoReg,
	}
}
func (cr *CommunityRepoImporter) LoadAndSave(ctx context.Context) error {
	repos, err := cr.loadFromFile()
	if err != nil {
		return err
	}

	for _, r := range repos {
		_, err = cr.daoReg.RepositoryConfig.InternalOnly_RefreshCommunityRepo(ctx, r.ToRepositoryRequest())
		if err != nil {
			return err
		}
	}
	return nil
}

func (cr *CommunityRepoImporter) loadFromFile() ([]CommunityRepo, error) {
	var (
		repos    []CommunityRepo
		contents []byte
		err      error
	)

	contents, err = crFS.ReadFile(CommunityReposFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(contents, &repos)
	if err != nil {
		return nil, err
	}

	filteredRepos := []CommunityRepo{}
	filter := config.Get().Options.RepositoryImportFilter
	filters := strings.Split(filter, ",")

	for _, repo := range repos {
		selectors := strings.Split(repo.Selector, ",")
		if filter == "" || utils.ContainsAny(filters, selectors) {
			filteredRepos = append(filteredRepos, repo)
		}
	}

	return filteredRepos, nil
}
