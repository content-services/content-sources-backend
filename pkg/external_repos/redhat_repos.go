package external_repos

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

const RedHatReposDirectory = "snapshotted_repos"
const RedHatGpgKeyFile = "redhat.gpg"
const RedHat10GpgKeyFile = "redhat_10.gpg"

//go:embed "redhat.gpg"
//go:embed "redhat_10.gpg"
//go:embed "snapshotted_repos/*"

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
	repos, err := rhr.loadFromFiles()
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

func (rhr *RedHatRepoImporter) loadFromFiles() ([]RedHatRepo, error) {
	files, err := rhFS.ReadDir(RedHatReposDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", RedHatReposDirectory, err)
	}
	var repos []RedHatRepo
	for _, file := range files {
		filename := path.Join(RedHatReposDirectory, file.Name())
		fileRepos, err := rhr.loadFromFile(filename)
		if err != nil {
			return repos, fmt.Errorf("failed to load file %s: %w", filename, err)
		}
		repos = append(repos, fileRepos...)
	}
	return repos, nil
}

func (rhr *RedHatRepoImporter) loadFromFile(filename string) ([]RedHatRepo, error) {
	var (
		repos    []RedHatRepo
		contents []byte
		err      error
	)

	contents, err = rhFS.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(contents, &repos)
	if err != nil {
		return nil, err
	}
	filteredRepos := []RedHatRepo{}
	filter := config.Get().Options.RepositoryImportFilter
	features := config.Get().Options.FeatureFilter
	for _, repo := range repos {
		selectors := strings.Split(repo.Selector, ",")
		if filter == "" || slices.Contains(selectors, filter) {
			if utils.Contains(features, repo.FeatureName) {
				filteredRepos = append(filteredRepos, repo)
			}
		}
	}
	return filteredRepos, nil
}
