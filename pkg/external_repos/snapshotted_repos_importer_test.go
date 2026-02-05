package external_repos

import (
	"encoding/json"
	"fmt"
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

	// Extract unique versions from both files
	extendedReleaseVersions := make(map[string]map[string]bool) // version -> set of feature names

	for _, repo := range e4sRepos {
		version := repo.ExtendedReleaseVersion
		if extendedReleaseVersions[version] == nil {
			extendedReleaseVersions[version] = make(map[string]bool)
		}
		extendedReleaseVersions[version][repo.FeatureName] = true
	}

	for _, repo := range eusRepos {
		version := repo.ExtendedReleaseVersion
		if extendedReleaseVersions[version] == nil {
			extendedReleaseVersions[version] = make(map[string]bool)
		}
		extendedReleaseVersions[version][repo.FeatureName] = true
	}

	// Verify each version from JSON files exists in DistributionMinorVersions with correct features
	for version, features := range extendedReleaseVersions {
		found := false
		for _, minorVersion := range config.DistributionMinorVersions {
			if minorVersion.Label == version {
				found = true
				// Verify that all features from JSON are present in the config
				for feature := range features {
					assert.Contains(t, minorVersion.FeatureNames, feature,
						fmt.Sprintf("Version %s should have feature %s in config", version, feature))
				}
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("Version %s from JSON files should exist in DistributionMinorVersions", version))
	}

	// Verify each version in DistributionMinorVersions exists in the JSON files with correct features
	for _, minorVersion := range config.DistributionMinorVersions {
		version := minorVersion.Label
		jsonFeatures, found := extendedReleaseVersions[version]
		assert.True(t, found, fmt.Sprintf("Version %s from config.DistributionMinorVersions should exist in JSON files", version))

		if found {
			// Verify that all features in config are present in the JSON files
			for _, configFeature := range minorVersion.FeatureNames {
				assert.True(t, jsonFeatures[configFeature],
					fmt.Sprintf("Version %s in config has feature %s, but it's not in the JSON files", version, configFeature))
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
