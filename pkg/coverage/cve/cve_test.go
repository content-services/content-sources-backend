package cve

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func javaResult(namespace, name, status string) matcher.MatchResult {
	return matcher.MatchResult{
		Package:     matcher.Package{Ecosystem: matcher.EcosystemJava, Namespace: namespace, Name: name, Version: "1.0.0"},
		MatchStatus: status,
	}
}

func pythonResult(name, status string) matcher.MatchResult {
	return matcher.MatchResult{
		Package:     matcher.Package{Ecosystem: matcher.EcosystemPython, Name: name, Version: "1.0.0"},
		MatchStatus: status,
	}
}

func TestEnrichBucketsBySeverityThresholds(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.0},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-2", SeverityScore: 8.9},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-3", SeverityScore: 7.0},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-4", SeverityScore: 6.9},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-5", SeverityScore: 4.0},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-6", SeverityScore: 3.9},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-7", SeverityScore: 0.1},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1, High: 2, Medium: 2, Low: 2}, perPackage[0].Count)
	assert.Equal(t, Count{Critical: 1, High: 2, Medium: 2, Low: 2}, summary)
}

func TestEnrichComputesRange(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-2", SeverityScore: 1.2},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-3", SeverityScore: 5.5},
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	require.NotNil(t, perPackage[0].Range)
	assert.Equal(t, float32(1.2), perPackage[0].Range.Low)
	assert.Equal(t, float32(9.8), perPackage[0].Range.High)
}

func TestEnrichExcludesZeroScore(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 0},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count)
	assert.Nil(t, perPackage[0].Range, "a CVE with no score should not produce a range")
	assert.Equal(t, Count{}, summary)
}

func TestEnrichSkipsUnmatchedPackages(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusNone)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count)
	assert.Nil(t, perPackage[0].Range)
	assert.Equal(t, Count{}, summary)
}

func TestEnrichIncludesPartialMatches(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusPartial)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count)
}

func TestEnrichDedupesByAdvisoryID(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count)
}

func TestEnrichMatchesJavaCaseInsensitively(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.Example", "Lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count, "java package should match advisory regardless of case")
}

func TestEnrichMatchesPythonWithNormalization(t *testing.T) {
	results := []matcher.MatchResult{pythonResult("flask-login", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemPython, PackageName: "Flask_Login", AdvisoryID: "CVE-1", SeverityScore: 7.5},
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{High: 1}, perPackage[0].Count, "python names should match after PEP 503 normalization")
}

func TestEnrichAggregatesAcrossPackages(t *testing.T) {
	results := []matcher.MatchResult{
		javaResult("com.example", "lib", matcher.MatchStatusExact),
		pythonResult("flask", matcher.MatchStatusExact),
	}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8},
		{Ecosystem: matcher.EcosystemPython, PackageName: "flask", AdvisoryID: "CVE-2", SeverityScore: 7.5},
		{Ecosystem: matcher.EcosystemPython, PackageName: "flask", AdvisoryID: "CVE-3", SeverityScore: 4.0},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 2)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count)
	assert.Equal(t, Count{High: 1, Medium: 1}, perPackage[1].Count)
	assert.Equal(t, Count{Critical: 1, High: 1, Medium: 1}, summary)
}

func TestEnrichPackageWithNoAdvisories(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}

	perPackage, summary := Enrich(results, nil)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count)
	assert.Nil(t, perPackage[0].Range)
	assert.Equal(t, Count{}, summary)
}
