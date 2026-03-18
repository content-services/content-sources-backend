package external_repos

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SnapshotRepoImporterSuite struct {
	suite.Suite
}

func TestSnapshotRepoImporterSuite(t *testing.T) {
	suite.Run(t, new(SnapshotRepoImporterSuite))
}

func (s *SnapshotRepoImporterSuite) TestDistributionMinorVersionsMatchExtendedReleaseRepos() {
	t := s.T()

	// Read e4s and eus repository sets from embedded files
	e4sRepos := readEmbeddedExtendedReleaseRepos(t, "snapshotted_repos/e4s-x86_64.json")
	eusRepos := readEmbeddedExtendedReleaseRepos(t, "snapshotted_repos/eus-x86_64.json")
	eusReposARM := readEmbeddedExtendedReleaseRepos(t, "snapshotted_repos/eus-aarch64.json")
	eeusRepos := readEmbeddedExtendedReleaseRepos(t, "snapshotted_repos/eeus-x86_64.json")
	eeusReposARM := readEmbeddedExtendedReleaseRepos(t, "snapshotted_repos/eeus-aarch64.json")

	// Extract unique versions from both files
	extendedReleaseVersions := make(map[string]map[string]bool) // version -> set of stream labels (e4s, eus)

	for _, repo := range slices.Concat(e4sRepos, eusRepos, eusReposARM, eeusRepos, eeusReposARM) {
		version := repo.ExtendedReleaseVersion
		if extendedReleaseVersions[version] == nil {
			extendedReleaseVersions[version] = make(map[string]bool)
		}
		extendedReleaseVersions[version][repo.ExtendedRelease] = true
	}

	// Verify each version from JSON files exists in DistributionMinorVersions with correct streams
	for version, streams := range extendedReleaseVersions {
		found := false
		for _, minorVersion := range config.DistributionMinorVersions {
			if minorVersion.Label == version {
				found = true
				// Verify that all streams from JSON are present in the config
				for stream := range streams {
					assert.Contains(t, minorVersion.ExtendedReleaseStreams, stream,
						fmt.Sprintf("Version %s should have stream %s in config", version, stream))
				}
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("Version %s from JSON files should exist in DistributionMinorVersions", version))
	}

	// Verify each version in DistributionMinorVersions exists in the JSON files with correct streams
	for _, minorVersion := range config.DistributionMinorVersions {
		version := minorVersion.Label
		jsonStreams, found := extendedReleaseVersions[version]
		assert.True(t, found, fmt.Sprintf("Version %s from config.DistributionMinorVersions should exist in JSON files", version))

		if found {
			// Verify that all streams in config are present in the JSON files
			for _, configStream := range minorVersion.ExtendedReleaseStreams {
				assert.True(t, jsonStreams[configStream],
					fmt.Sprintf("Version %s in config has stream %s, but it's not in the JSON files", version, configStream))
			}
		}
	}
}

func readEmbeddedExtendedReleaseRepos(t *testing.T, filePath string) []SnapshottedRepo {
	data, err := rhFS.ReadFile(filePath)
	assert.Nil(t, err, fmt.Sprintf("Failed to read %s", filePath))

	var repos []SnapshottedRepo
	err = json.Unmarshal(data, &repos)
	assert.Nil(t, err, fmt.Sprintf("Failed to unmarshal %s", filePath))

	return repos
}
