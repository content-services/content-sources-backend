package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublishedOnNetwork(t *testing.T) {
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
	}

	assert.True(t, publishedOnNetwork(match, []PublishedAdvisory{advisory}))
	assert.False(t, publishedOnNetwork(Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
	}, []PublishedAdvisory{advisory}))
	assert.False(t, publishedOnNetwork(Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:other-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
	}, []PublishedAdvisory{advisory}))
	assert.False(t, publishedOnNetwork(Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2",
		Language:         &java,
	}, []PublishedAdvisory{advisory}))
	python := "python"
	assert.False(t, publishedOnNetwork(Vulnerability{
		VulnerabilityID:  "CVE-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &python,
	}, []PublishedAdvisory{advisory}))
	assert.True(t, publishedOnNetwork(Vulnerability{
		VulnerabilityID:  "CVE-0000-0002",
		ComponentName:    "demo-pkg",
		ComponentVersion: "4.0.0",
		Language:         &python,
	}, []PublishedAdvisory{{
		RepoName:      "lightwell/python/validated",
		AdvisoryID:    "x_DEMO-CVE-0000-0002-4.0.0",
		PackageName:   "demo-pkg",
		FixedVersions: []string{"4.0.0"},
	}}))
	assert.False(t, publishedOnNetwork(match, []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-00010-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}}))
	assert.False(t, publishedOnNetwork(Vulnerability{
		VulnerabilityID:  "LW-0000-0001",
		ComponentName:    "com.example:demo-lib",
		ComponentVersion: "1.2.3",
		Language:         &java,
	}, []PublishedAdvisory{{
		RepoName:      "java/remediated",
		AdvisoryID:    "x_DEMO-LW-0000-00010-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}}))
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

	inProgress := match
	inProgress.Stage = "Fix in Progress"
	applyPublishedStage(&inProgress, advisories)
	assert.Equal(t, "Fix in Progress", inProgress.Stage)

	closedUnpublished := match
	closedUnpublished.Stage = "Validation"
	closedUnpublished.ComponentName = "com.example:other-lib"
	applyPublishedStage(&closedUnpublished, advisories)
	assert.Equal(t, "Validation", closedUnpublished.Stage)
}

func TestPublishedOnNetworkMatchesCaseInsensitiveMavenArtifactName(t *testing.T) {
	java := "java"
	purl := "pkg:maven/com.example/demo-cli@1.2.3"
	vulnerability := Vulnerability{
		VulnerabilityID:  "LW-0000-0101",
		PURL:             &purl,
		ComponentName:    "DEMO-CLI",
		ComponentVersion: "1.2.3",
		Language:         &java,
	}
	advisory := PublishedAdvisory{
		RepoName:      "demo/java/predisclosure",
		AdvisoryID:    "x_DEMO-LW-0000-0101-1.2.3",
		PackageName:   "COM.EXAMPLE:DEMO-CLI",
		FixedVersions: []string{"1.2.3.demo-00001"},
	}

	assert.True(t, publishedOnNetwork(vulnerability, []PublishedAdvisory{advisory}))
}

func TestPublishedOnNetworkDoesNotMatchPackagePrefix(t *testing.T) {
	java := "java"
	purl := "pkg:maven/com.example/demo-web@2.3.4"
	vulnerability := Vulnerability{
		VulnerabilityID:  "CVE-0000-0102",
		PURL:             &purl,
		ComponentName:    "com.example:demo-web",
		ComponentVersion: "2.3.4",
		Language:         &java,
	}
	advisory := PublishedAdvisory{
		RepoName:      "demo/java/remediated",
		AdvisoryID:    "x_DEMO-CVE-0000-0102-2.3.4",
		PackageName:   "com.example:demo-webmvc",
		FixedVersions: []string{"2.3.4.demo-00001"},
	}

	assert.False(t, publishedOnNetwork(vulnerability, []PublishedAdvisory{advisory}))
}

func TestPublishedOnNetworkUsesPURLIdentityForNonMavenPackages(t *testing.T) {
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
			}
			advisory := PublishedAdvisory{
				RepoName:      test.repoName,
				AdvisoryID:    "x_DEMO-CVE-0000-0103-1.0.0",
				PackageName:   test.packageName,
				FixedVersions: []string{"1.0.0.demo-00001"},
			}

			assert.True(t, publishedOnNetwork(vulnerability, []PublishedAdvisory{advisory}))
		})
	}
}
