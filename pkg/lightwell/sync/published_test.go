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
