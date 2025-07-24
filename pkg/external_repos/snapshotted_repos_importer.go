package external_repos

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

const SnapshottedReposDirectory = "snapshotted_repos"
const RedHatGpgKeyFile = "redhat.gpg"
const RedHat10GpgKeyFile = "redhat_10.gpg"

//go:embed "redhat.gpg"
//go:embed "redhat_10.gpg"
//go:embed "snapshotted_repos/*"

var rhFS embed.FS

type SnapshottedRepo struct {
	Url                 string `json:"url"`
	Name                string `json:"name"`
	DistributionArch    string `json:"distribution_arch"`
	DistributionVersion string `json:"distribution_version"`
	Selector            string `json:"selector"`
	GpgKey              string `json:"gpg_key"`
	Label               string `json:"content_label"`
	FeatureName         string `json:"feature_name"`
	Origin              string `json:"origin"`
}

func (rhr SnapshottedRepo) ToRepositoryRequest() api.RepositoryRequest {
	return api.RepositoryRequest{
		Name:                 &rhr.Name,
		URL:                  &rhr.Url,
		DistributionVersions: &[]string{rhr.DistributionVersion},
		DistributionArch:     &rhr.DistributionArch,
		GpgKey:               &rhr.GpgKey,
		MetadataVerification: utils.Ptr(false),
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(rhr.Origin),
		ContentType:          utils.Ptr(config.ContentTypeRpm),
	}
}

type SnapshotRepoImporter struct {
	daoReg *dao.DaoRegistry
}

func NewSnapshotRepoImporter(daoReg *dao.DaoRegistry) SnapshotRepoImporter {
	return SnapshotRepoImporter{
		daoReg: daoReg,
	}
}
func (rhr *SnapshotRepoImporter) LoadAndSave(ctx context.Context) error {
	repos, err := rhr.loadFromFiles()
	if err != nil {
		return err
	}

	for _, r := range repos {
		if r.Origin == config.OriginRedHat {
			gpgKey, err := redHatGpgKey(r.DistributionVersion)
			if err != nil {
				return err
			}
			r.GpgKey = gpgKey
		}
		_, err = rhr.daoReg.RepositoryConfig.InternalOnly_RefreshPredefinedSnapshotRepo(ctx, r.ToRepositoryRequest(), r.Label, r.FeatureName)
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

func (rhr *SnapshotRepoImporter) loadFromFiles() ([]SnapshottedRepo, error) {
	files, err := rhFS.ReadDir(SnapshottedReposDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", SnapshottedReposDirectory, err)
	}
	var repos []SnapshottedRepo
	for _, file := range files {
		filename := path.Join(SnapshottedReposDirectory, file.Name())
		fileRepos, err := rhr.loadFromFile(filename)
		if err != nil {
			return repos, fmt.Errorf("failed to load file %s: %w", filename, err)
		}
		repos = append(repos, fileRepos...)
	}
	return repos, nil
}

func (rhr *SnapshotRepoImporter) loadFromFile(filename string) ([]SnapshottedRepo, error) {
	var (
		repos    []SnapshottedRepo
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
	filteredRepos := []SnapshottedRepo{}
	filter := config.Get().Options.RepositoryImportFilter
	filters := strings.Split(filter, ",")
	features := config.Get().Options.FeatureFilter
	features = append(features, "RHEL-OS-x86_64")
	for _, repo := range repos {
		selectors := strings.Split(repo.Selector, ",")
		if filter == "" || utils.ContainsAny(filters, selectors) {
			// If the repo is not from Red Hat or if it matches one of the features, include it
			if repo.Origin != config.OriginRedHat || utils.Contains(features, repo.FeatureName) {
				filteredRepos = append(filteredRepos, repo)
			}
		}
	}
	return filteredRepos, nil
}
