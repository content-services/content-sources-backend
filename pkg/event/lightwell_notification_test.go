package event

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCVSSScoreToLabel(t *testing.T) {
	tests := []struct {
		score    string
		expected string
	}{
		{"9.8", SeverityCritical},
		{"9.0", SeverityCritical},
		{"8.5", SeverityImportant},
		{"7.0", SeverityImportant},
		{"6.9", SeverityModerate},
		{"4.0", SeverityModerate},
		{"3.9", SeverityLow},
		{"0.1", SeverityLow},
		{"0.0", SeverityLow},
		{"", SeverityLow},
		{"invalid", SeverityLow},
	}

	for _, tt := range tests {
		t.Run(tt.score, func(t *testing.T) {
			assert.Equal(t, tt.expected, CVSSScoreToLabel(tt.score))
		})
	}
}

func TestMaximumSeverity(t *testing.T) {
	data := []LightwellNotificationInput{
		{Severity: "3.5"},
		{Severity: "9.1"},
		{Severity: "7.5"},
	}
	assert.Equal(t, SeverityCritical, MaximumSeverity(data))

	data = []LightwellNotificationInput{
		{Severity: "3.5"},
		{Severity: "6.0"},
	}
	assert.Equal(t, SeverityModerate, MaximumSeverity(data))

	data = []LightwellNotificationInput{
		{Severity: "2.0"},
	}
	assert.Equal(t, SeverityLow, MaximumSeverity(data))
}

func TestMaximumSeverityEmpty(t *testing.T) {
	assert.Equal(t, SeverityLow, MaximumSeverity(nil))
}

func TestLightwellEventType(t *testing.T) {
	assert.Equal(t, LightwellEventTypeJavaRemediated, LightwellEventType("lightwell/java/remediated"))
	assert.Equal(t, "", LightwellEventType("lightwell/java/validated"))
	assert.Equal(t, "", LightwellEventType("unknown"))
}

func TestBuildLightwellNotificationEventsGroupsByPackage(t *testing.T) {
	// Input: 4 advisories across 2 packages
	//   lib-a: CVE-0001 and CVE-0002 share fixed versions [1.0.1, 1.0.2]
	//          CVE-0003 has a different fixed version [1.0.3]
	//   lib-b: CVE-0004 is the only advisory
	//
	// Expected output: 2 events (one per package)
	//   lib-a event: 2 releases (one for the shared versions, one for 1.0.3)
	//   lib-b event: 1 release
	inputs := []LightwellNotificationInput{
		{
			PackageName:   "org.example:lib-a",
			AdvisoryID:    "CVE-2026-0001",
			Severity:      "9.8",
			FixedVersions: []string{"1.0.1", "1.0.2"},
		},
		{
			PackageName:   "org.example:lib-a",
			AdvisoryID:    "CVE-2026-0002",
			Severity:      "7.5",
			FixedVersions: []string{"1.0.1", "1.0.2"},
		},
		{
			PackageName:   "org.example:lib-a",
			AdvisoryID:    "CVE-2026-0003",
			Severity:      "4.0",
			FixedVersions: []string{"1.0.3"},
		},
		{
			PackageName:   "org.example:lib-b",
			AdvisoryID:    "CVE-2026-0004",
			Severity:      "6.5",
			FixedVersions: []string{"2.0.0"},
		},
	}

	events := BuildLightwellNotificationEvents(inputs)
	require.Len(t, events, 2)

	// Index by package name to avoid depending on map iteration order
	payloadsByPkg := make(map[string]LightwellPackagePayload)
	for _, e := range events {
		p, ok := e.Payload.(LightwellPackagePayload)
		require.True(t, ok)
		payloadsByPkg[p.PackageName] = p
	}

	// lib-a: 3 advisories should produce 2 releases (grouped by shared fixed versions)
	libA := payloadsByPkg["org.example:lib-a"]
	assert.Equal(t, LightwellPackageLink("org.example:lib-a"), libA.PackageLink)
	require.Len(t, libA.Releases, 2)

	// Index releases by their version names to avoid depending on map iteration order
	releasesByKey := make(map[string]LightwellReleasePayload)
	for _, r := range libA.Releases {
		var names []string
		for _, n := range r.ReleaseNames {
			names = append(names, n.Name)
		}
		releasesByKey[strings.Join(names, ",")] = r
	}

	// CVE-0001 (critical) and CVE-0002 (important) share fixed versions 1.0.1 and 1.0.2
	sharedRelease := releasesByKey["1.0.1,1.0.2"]
	require.Len(t, sharedRelease.RelatedCVE, 2)
	assert.Equal(t, "CVE-2026-0001", sharedRelease.RelatedCVE[0].CVE)
	assert.Equal(t, SeverityCritical, sharedRelease.RelatedCVE[0].Severity)
	assert.Equal(t, LightwellCVEURL("CVE-2026-0001"), sharedRelease.RelatedCVE[0].URL)
	assert.Equal(t, "CVE-2026-0002", sharedRelease.RelatedCVE[1].CVE)
	assert.Equal(t, SeverityImportant, sharedRelease.RelatedCVE[1].Severity)

	// CVE-0003 (moderate) has its own fixed version 1.0.3
	soloRelease := releasesByKey["1.0.3"]
	require.Len(t, soloRelease.RelatedCVE, 1)
	assert.Equal(t, "CVE-2026-0003", soloRelease.RelatedCVE[0].CVE)
	assert.Equal(t, SeverityModerate, soloRelease.RelatedCVE[0].Severity)

	// lib-b: single advisory, single release
	libB := payloadsByPkg["org.example:lib-b"]
	assert.Equal(t, LightwellPackageLink("org.example:lib-b"), libB.PackageLink)
	require.Len(t, libB.Releases, 1)
	require.Len(t, libB.Releases[0].RelatedCVE, 1)
	assert.Equal(t, "CVE-2026-0004", libB.Releases[0].RelatedCVE[0].CVE)
}

func TestBuildLightwellNotificationEventsEmpty(t *testing.T) {
	events := BuildLightwellNotificationEvents(nil)
	assert.Empty(t, events)
}

func TestBuildLightwellNotificationEventsMetadata(t *testing.T) {
	inputs := []LightwellNotificationInput{
		{
			PackageName:   "org.example:lib-a",
			AdvisoryID:    "CVE-2026-0001",
			Severity:      "9.8",
			FixedVersions: []string{"1.0.1"},
		},
	}

	events := BuildLightwellNotificationEvents(inputs)
	require.Len(t, events, 1)
	assert.NotNil(t, events[0].Metadata)
	assert.Empty(t, events[0].Metadata)
}
