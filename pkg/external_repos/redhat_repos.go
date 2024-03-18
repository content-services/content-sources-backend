package external_repos

import (
	"context"
	"embed"
	"encoding/json"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/openlyinc/pointy"
)

const RedHatReposFile = "redhat_repos.json"
const RedHatGpgKeyFile = "redhat.gpg"

//go:embed "redhat_repos.json"
//go:embed "redhat.gpg"

var rhFS embed.FS

type RedHatRepo struct {
	Url                 string `json:"url"`
	Name                string `json:"name"`
	Arch                string `json:"arch"`
	DistributionVersion string `json:"distribution_version"`
	Selector            string `json:"selector"`
	GpgKey              string `json:"gpg_key"`
	Label               string `json:"content_label"`
}

func (rhr RedHatRepo) ToRepositoryRequest() api.RepositoryRequest {
	return api.RepositoryRequest{
		Name:                 &rhr.Name,
		URL:                  &rhr.Url,
		DistributionVersions: &[]string{rhr.DistributionVersion},
		DistributionArch:     &rhr.Arch,
		GpgKey:               &rhr.GpgKey,
		MetadataVerification: pointy.Pointer(false),
		Origin:               pointy.Pointer(config.OriginRedHat),
		ContentType:          pointy.Pointer(config.ContentTypeRpm),
		Snapshot:             pointy.Pointer(true),
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
	gpgKey, err := redHatGpgKey()
	if err != nil {
		return err
	}
	for _, r := range repos {
		r.GpgKey = gpgKey
		_, err = rhr.daoReg.RepositoryConfig.InternalOnly_RefreshRedHatRepo(ctx, r.ToRepositoryRequest(), r.Label)
		if err != nil {
			return err
		}
	}
	return nil
}

func redHatGpgKey() (string, error) {
	contents, err := rhFS.ReadFile(RedHatGpgKeyFile)
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
	for _, repo := range repos {
		if filter == "" || repo.Selector == filter {
			filteredRepos = append(filteredRepos, repo)
		}
	}
	return filteredRepos, nil
}
