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

const RedHatReposFile = "redhat_repos.json"
const RedHatGpgKeyFile = "redhat.gpg"
const RedHat10GpgKeyFile = "redhat_10.gpg"

//go:embed "redhat_repos.json"
//go:embed "redhat.gpg"
//go:embed "redhat_10.gpg"

var rhFS embed.FS

type RedHatRepo struct {
	Url                 string `json:"url"`
	Name                string `json:"name"`
	Arch                string `json:"arch"`
	DistributionVersion string `json:"distribution_version"`
	Selector            string `json:"selector"`
	GpgKey              string `json:"gpg_key"`
	Label               string `json:"content_label"`
	FeatureName         string `json:"feature_name"`
}

func (rhr RedHatRepo) ToRepositoryRequest() api.RepositoryRequest {
	return api.RepositoryRequest{
		Name:                 &rhr.Name,
		URL:                  &rhr.Url,
		DistributionVersions: &[]string{rhr.DistributionVersion},
		DistributionArch:     &rhr.Arch,
		GpgKey:               &rhr.GpgKey,
		MetadataVerification: utils.Ptr(false),
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginRedHat),
		ContentType:          utils.Ptr(config.ContentTypeRpm),
	}
}

type RedHatRepoImporter struct {
	daoReg *dao.DaoRegistry
}

func NewRedHatRepos(daoReg *dao.DaoRegistry) RedHatRepoImporter {
	return RedHatRepoImporter{
		daoReg: daoReg,
	}
}
func (rhr *RedHatRepoImporter) LoadAndSave(ctx context.Context) error {
	repos, err := rhr.loadFromFile()
	if err != nil {
		return err
	}

	for _, r := range repos {
		gpgKey, err := redHatGpgKey(r.DistributionVersion)
		if err != nil {
			return err
		}
		r.GpgKey = gpgKey
		_, err = rhr.daoReg.RepositoryConfig.InternalOnly_RefreshRedHatRepo(ctx, r.ToRepositoryRequest(), r.Label, r.FeatureName)
		if err != nil {
			return err
		}
	}
	return nil
}

func redHatGpgKey(version string) (string, error) {
	file := RedHatGpgKeyFile
	if version == "10" {
		file = RedHat10GpgKeyFile
	}
	contents, err := rhFS.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

func (rhr *RedHatRepoImporter) loadFromFile() ([]RedHatRepo, error) {
	var (
		repos    []RedHatRepo
		contents []byte
		err      error
	)

	contents, err = rhFS.ReadFile(RedHatReposFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(contents, &repos)
	if err != nil {
		return nil, err
	}
	filteredRepos := []RedHatRepo{}
	filter := config.Get().Options.RepositoryImportFilter
	filters := strings.Split(filter, ",")
	features := config.Get().Options.FeatureFilter
	for _, repo := range repos {
		selectors := strings.Split(repo.Selector, ",")
		if filter == "" || utils.ContainsAny(filters, selectors) {
			if utils.Contains(features, repo.FeatureName) {
				filteredRepos = append(filteredRepos, repo)
			}
		}
	}
	return filteredRepos, nil
}
