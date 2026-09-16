package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateIfPublishedOnNetwork(t *testing.T) {
	java := "java"
	advisory := PublishedAdvisory{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-0001-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}
	match := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}

	updateIfPublishedOnNetwork(&match, []PublishedAdvisory{advisory})
	assert.Equal(t, []string{"1.2.3.build-00001"}, match.PublishedVersions)
	assert.Equal(t, "Lightwell Network", match.Stage)

	missingLanguage := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&missingLanguage, []PublishedAdvisory{advisory})
	assert.Empty(t, missingLanguage.PublishedVersions)
	assert.Equal(t, "Validation", missingLanguage.Stage)

	otherLib := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:other-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&otherLib, []PublishedAdvisory{advisory})
	assert.Empty(t, otherLib.PublishedVersions)
	assert.Equal(t, "Validation", otherLib.Stage)

	shortVersion := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2",
		Language:         &java,
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&shortVersion, []PublishedAdvisory{advisory})
	assert.Empty(t, shortVersion.PublishedVersions)
	assert.Equal(t, "Validation", shortVersion.Stage)

	python := "python"
	wrongLanguage := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &python,
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&wrongLanguage, []PublishedAdvisory{advisory})
	assert.Empty(t, wrongLanguage.PublishedVersions)
	assert.Equal(t, "Validation", wrongLanguage.Stage)

	pythonMatch := Vulnerability{
		VulnerabilityID:  "CVE-0000-0002",
		ComponentName:    "demo-pkg",
		ComponentVersion: "4.0.0",
		Language:         &python,
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&pythonMatch, []PublishedAdvisory{{
		RepoName:      "lightwell/python/validated",
		AdvisoryID:    "x_DEMO-CVE-0000-0002-4.0.0",
		PackageName:   "demo-pkg",
		FixedVersions: []string{"4.0.0"},
	}})
	assert.Equal(t, []string{"4.0.0"}, pythonMatch.PublishedVersions)
	assert.Equal(t, "Lightwell Network", pythonMatch.Stage)

	longerID := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&longerID, []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-00010-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}})
	assert.Empty(t, longerID.PublishedVersions)
	assert.Equal(t, "Validation", longerID.Stage)

	lwPrefix := Vulnerability{
		VulnerabilityID:  "LW-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}
	updateIfPublishedOnNetwork(&lwPrefix, []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-LW-0000-00010-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}})
	assert.Empty(t, lwPrefix.PublishedVersions)
	assert.Equal(t, "Validation", lwPrefix.Stage)
}

func TestUpdateIfPublishedOnNetworkCollectsUniqueSortedVersions(t *testing.T) {
	java := "java"
	match := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}

	updateIfPublishedOnNetwork(&match, []PublishedAdvisory{
		{
			RepoName:      "java/predisclosure",
			AdvisoryID:    "x_DEMO-CVE-0000-0001-1.2.3",
			PackageName:   "com.example:demo-lib",
			FixedVersions: []string{"1.2.3.build-00002", "9.9.9", "1.2.3.build-00001"},
		},
		{
			RepoName:      "java/remediated",
			AdvisoryID:    "x_DEMO-CVE-0000-0001-1.2.3",
			PackageName:   "com.example:demo-lib",
			FixedVersions: []string{"1.2.3.build-00001"},
		},
	})
	assert.Equal(t, []string{"1.2.3.build-00002", "1.2.3.build-00001"}, match.PublishedVersions)
	assert.Equal(t, "Lightwell Network", match.Stage)
}

func TestUpdateIfPublishedOnNetworkIgnoresMatchWithoutFixedVersions(t *testing.T) {
	java := "java"
	vulnerability := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}

	updateIfPublishedOnNetwork(&vulnerability, []PublishedAdvisory{{
		RepoName:    "java/remediated",
		AdvisoryID:  "x_DEMO-CVE-0000-0001-1.2.3",
		PackageName: "com.example:demo-lib",
	}})
	assert.Empty(t, vulnerability.PublishedVersions)
	assert.Equal(t, "Validation", vulnerability.Stage)
}

func TestUpdateIfPublishedOnNetworkMatchesFixedVersionsWithoutIDSuffix(t *testing.T) {
	java := "java"
	vulnerability := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}

	updateIfPublishedOnNetwork(&vulnerability, []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-0001",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}})
	assert.Equal(t, []string{"1.2.3.build-00001"}, vulnerability.PublishedVersions)
	assert.Equal(t, "Lightwell Network", vulnerability.Stage)
}

func TestUpdateIfPublishedOnNetworkSkipsBlankFixedVersions(t *testing.T) {
	java := "java"
	vulnerability := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}

	updateIfPublishedOnNetwork(&vulnerability, []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-0001-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"", "  ", "1.2.3.build-00001"},
	}})
	assert.Equal(t, []string{"1.2.3.build-00001"}, vulnerability.PublishedVersions)
	assert.Equal(t, "Lightwell Network", vulnerability.Stage)
}

func TestApplyPublishedStageOnlyPromotesValidation(t *testing.T) {
	java := "java"
	match := Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}
	advisories := []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-0001-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}}

	applyPublishedStage(&match, advisories)
	assert.Equal(t, "Lightwell Network", match.Stage)
	assert.Equal(t, []string{"1.2.3.build-00001"}, match.PublishedVersions)

	inProgress := match
	inProgress.Stage = "Fix in Progress"
	applyPublishedStage(&inProgress, advisories)
	assert.Equal(t, "Fix in Progress", inProgress.Stage)
	assert.Equal(t, []string{"1.2.3.build-00001"}, inProgress.PublishedVersions)

	closedUnpublished := match
	closedUnpublished.Stage = "Validation"
	closedUnpublished.ComponentName = "com.example:other-lib"
	closedUnpublished.PublishedVersions = nil
	applyPublishedStage(&closedUnpublished, advisories)
	assert.Equal(t, "Validation", closedUnpublished.Stage)
	assert.Empty(t, closedUnpublished.PublishedVersions)
}

func TestUpdateIfPublishedOnNetworkMatchesCaseInsensitiveMavenArtifactName(t *testing.T) {
	java := "java"
	purl := "pkg:maven/com.example/demo-cli@1.2.3"
	vulnerability := Vulnerability{
		VulnerabilityID:  "LW-0000-0101",
		PURL:             &purl,
		ComponentName:    "DEMO-CLI",
		ComponentVersion: "1.2.3",
		Language:         &java,
		Stage:            "Validation",
	}
	advisory := PublishedAdvisory{
		RepoName:      "demo/java/predisclosure",
		AdvisoryID:    "x_DEMO-LW-0000-0101-1.2.3",
		PackageName:   "COM.EXAMPLE:DEMO-CLI",
		FixedVersions: []string{"1.2.3.demo-00001"},
	}

	updateIfPublishedOnNetwork(&vulnerability, []PublishedAdvisory{advisory})
	assert.Equal(t, []string{"1.2.3.demo-00001"}, vulnerability.PublishedVersions)
	assert.Equal(t, "Lightwell Network", vulnerability.Stage)
}

func TestUpdateIfPublishedOnNetworkDoesNotMatchPackagePrefix(t *testing.T) {
	java := "java"
	purl := "pkg:maven/com.example/demo-web@2.3.4"
	vulnerability := Vulnerability{
		VulnerabilityID:  "CVE-0000-0102",
		PURL:             &purl,
		ComponentName:    "com.example:demo-web",
		ComponentVersion: "2.3.4",
		Language:         &java,
		Stage:            "Validation",
	}
	advisory := PublishedAdvisory{
		RepoName:      "demo/java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-0102-2.3.4",
		PackageName:   "com.example:demo-webmvc",
		FixedVersions: []string{"2.3.4.demo-00001"},
	}

	updateIfPublishedOnNetwork(&vulnerability, []PublishedAdvisory{advisory})
	assert.Empty(t, vulnerability.PublishedVersions)
	assert.Equal(t, "Validation", vulnerability.Stage)
}

func TestUpdateIfPublishedOnNetworkUsesPURLIdentityForNonMavenPackages(t *testing.T) {
	tests := []struct {
		name          string
		language      string
		purl          string
		componentName string
		packageName   string
		repoName      string
	}{
		{
			name:          "PyPI",
			language:      "python",
			purl:          "pkg:pypi/demo-requests@2.31.0",
			componentName: "Demo Requests",
			packageName:   "demo-requests",
			repoName:      "demo/python/validated",
		},
		{
			name:          "npm scoped package",
			language:      "javascript",
			purl:          "pkg:npm/%40demo/core@15.0.0",
			componentName: "core",
			packageName:   "@demo/core",
			repoName:      "demo/javascript/validated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			purl := test.purl
			language := test.language
			vulnerability := Vulnerability{
				VulnerabilityID:  "CVE-0000-0103",
				PURL:             &purl,
				ComponentName:    test.componentName,
				ComponentVersion: "1.0.0",
				Language:         &language,
				Stage:            "Validation",
			}
			advisory := PublishedAdvisory{
				RepoName:      test.repoName,
				AdvisoryID:    "x_DEMO-CVE-0000-0103-1.0.0",
				PackageName:   test.packageName,
				FixedVersions: []string{"1.0.0.demo-00001"},
			}

			updateIfPublishedOnNetwork(&vulnerability, []PublishedAdvisory{advisory})
			assert.Equal(t, []string{"1.0.0.demo-00001"}, vulnerability.PublishedVersions)
			assert.Equal(t, "Lightwell Network", vulnerability.Stage)
		})
	}
}
