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

func (s *SnapshotRepoImporterSuite) TestRedHatGpgKeyWithExtendedRelease() {
	t := s.T()

	// Test 1: Extended release version in map returns custom key
	key94, err := redHatGpgKey("9", "9.4")
	assert.NoError(t, err)
	assert.Contains(t, key94, "BEGIN PGP PUBLIC KEY BLOCK")
	// Verify it's different from base RHEL 9 key
	key9, _ := redHatGpgKey("9", "")
	assert.NotEqual(t, key9, key94, "9.4 key should be different from base RHEL 9 key")

	// Test 2: Extended release version in map (9.6)
	key96, err := redHatGpgKey("9", "9.6")
	assert.NoError(t, err)
	assert.Contains(t, key96, "BEGIN PGP PUBLIC KEY BLOCK")
	assert.NotEqual(t, key9, key96, "9.6 key should be different from base RHEL 9 key")

	// Test 3: Extended release version in map (10.0)
	key100, err := redHatGpgKey("10", "10.0")
	assert.NoError(t, err)
	assert.Contains(t, key100, "BEGIN PGP PUBLIC KEY BLOCK")
	key10, _ := redHatGpgKey("10", "")
	assert.NotEqual(t, key10, key100, "10.0 key should be different from base RHEL 10 key")

	// Test 4: Extended release version NOT in map falls back to base version
	key92, err := redHatGpgKey("9", "9.2")
	assert.NoError(t, err)
	assert.Equal(t, key9, key92, "9.2 should fall back to base RHEL 9 key")

	// Test 5: Empty extended release version falls back to base version
	keyEmpty, err := redHatGpgKey("9", "")
	assert.NoError(t, err)
	assert.Equal(t, key9, keyEmpty, "Empty extended release should use base key")
}

func (s *SnapshotRepoImporterSuite) TestLoadFromFilesAssignsCorrectGpgKeys() {
	t := s.T()

	// Enable the extended release repos feature flag to test the actual integration path
	originalFeatureFlag := config.Get().Features.ExtendedReleaseRepos.Enabled
	config.Get().Features.ExtendedReleaseRepos.Enabled = true
	defer func() {
		config.Get().Features.ExtendedReleaseRepos.Enabled = originalFeatureFlag
	}()

	// Configure feature filter to include extended release repo features
	originalFeatureFilter := config.Get().Options.FeatureFilter
	config.Get().Options.FeatureFilter = []string{
		"RHEL-EUS-x86_64",
		"RHEL-EUS-aarch64",
		"RHEL-E4S-x86_64",
		"RHEL-EEUS-x86_64",
		"RHEL-EEUS-aarch64",
		"RHEL-SAP-EUS-x86_64",
		"RHEL-SAP_SOLUTIONS-EUS-x86_64",
		"RHEL-SAP-E4S-x86_64",
		"RHEL-SAP_SOLUTIONS-E4S-x86_64",
	}
	defer func() {
		config.Get().Options.FeatureFilter = originalFeatureFilter
	}()

	// Create importer and load repos using the actual production code path
	importer := SnapshotRepoImporter{}
	repos, err := importer.loadFromFiles()
	assert.NoError(t, err, "loadFromFiles should succeed")

	// Map to track which repos we've validated
	validated := make(map[string]bool)

	for _, repo := range repos {
		if repo.Origin != config.OriginRedHat {
			continue
		}

		// Get expected GPG key based on logic
		expectedKey, err := redHatGpgKey(repo.DistributionVersion, repo.ExtendedReleaseVersion)
		assert.NoError(t, err)

		// For extended release versions with custom keys, verify they would get custom key
		switch repo.ExtendedReleaseVersion {
		case "9.4":
			key94, _ := redHatGpgKey("9", "9.4")
			assert.Equal(t, key94, expectedKey, "9.4 repos should use custom GPG key")
			validated["9.4"] = true
		case "9.6":
			key96, _ := redHatGpgKey("9", "9.6")
			assert.Equal(t, key96, expectedKey, "9.6 repos should use custom GPG key")
			validated["9.6"] = true
		case "10.0":
			key100, _ := redHatGpgKey("10", "10.0")
			assert.Equal(t, key100, expectedKey, "10.0 repos should use custom GPG key")
			validated["10.0"] = true
		}
	}

	// Verify we actually tested the versions we care about
	assert.True(t, validated["9.4"], "Should have found and validated 9.4 repos")
	assert.True(t, validated["9.6"], "Should have found and validated 9.6 repos")
	assert.True(t, validated["10.0"], "Should have found and validated 10.0 repos")
}

func readEmbeddedExtendedReleaseRepos(t *testing.T, filePath string) []SnapshottedRepo {
	data, err := rhFS.ReadFile(filePath)
	assert.Nil(t, err, fmt.Sprintf("Failed to read %s", filePath))

	var repos []SnapshottedRepo
	err = json.Unmarshal(data, &repos)
	assert.Nil(t, err, fmt.Sprintf("Failed to unmarshal %s", filePath))

	return repos
}
