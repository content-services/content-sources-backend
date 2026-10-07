package cve

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers build packages at version 1.0.0; advisories in these tests fix 1.0.0 unless noted.
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

func javaAdvisory(id string, score float32) Advisory {
	return Advisory{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: id, SeverityScore: score, FixedVersions: []string{"1.0.0"}}
}

func TestEnrichBucketsBySeverityThresholds(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		javaAdvisory("CVE-1", 9.0),
		javaAdvisory("CVE-2", 8.9),
		javaAdvisory("CVE-3", 7.0),
		javaAdvisory("CVE-4", 6.9),
		javaAdvisory("CVE-5", 4.0),
		javaAdvisory("CVE-6", 3.9),
		javaAdvisory("CVE-7", 0.1),
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1, Important: 2, Moderate: 2, Low: 2}, perPackage[0].Count)
	assert.Equal(t, Count{Critical: 1, Important: 2, Moderate: 2, Low: 2}, summary)
}

func TestEnrichComputesRange(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		javaAdvisory("CVE-1", 9.8),
		javaAdvisory("CVE-2", 1.2),
		javaAdvisory("CVE-3", 5.5),
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	require.NotNil(t, perPackage[0].Range)
	assert.Equal(t, float32(1.2), perPackage[0].Range.Low)
	assert.Equal(t, float32(9.8), perPackage[0].Range.High)
}

func TestEnrichExcludesZeroScore(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{javaAdvisory("CVE-1", 0)}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count)
	assert.Nil(t, perPackage[0].Range, "a CVE with no score should not produce a range")
	assert.Equal(t, Count{}, summary)
}

func TestEnrichSkipsUnmatchedPackages(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusNone)}
	advisories := []Advisory{javaAdvisory("CVE-1", 9.8)}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count)
	assert.Nil(t, perPackage[0].Range)
	assert.Equal(t, Count{}, summary)
}

func TestEnrichExcludesPartialMatches(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusPartial)}
	advisories := []Advisory{javaAdvisory("CVE-1", 9.8)}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count, "partial matches are not version-scoped and must not get CVE data")
	assert.Nil(t, perPackage[0].Range)
	assert.Equal(t, Count{}, summary)
}

func TestEnrichScopesByVersion(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8, FixedVersions: []string{"2.0.0"}},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count, "an advisory that fixes a different version must not apply")
	assert.Nil(t, perPackage[0].Range)
	assert.Equal(t, Count{}, summary)
}

func TestEnrichDedupesByAdvisoryID(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}
	advisories := []Advisory{
		javaAdvisory("CVE-1", 9.8),
		javaAdvisory("CVE-1", 9.8),
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count)
}

func TestEnrichDedupesSummaryAcrossPackageVersions(t *testing.T) {
	results := []matcher.MatchResult{
		{Package: matcher.Package{Ecosystem: matcher.EcosystemJava, Namespace: "com.example", Name: "lib", Version: "1.0.0"}, MatchStatus: matcher.MatchStatusExact},
		{Package: matcher.Package{Ecosystem: matcher.EcosystemJava, Namespace: "com.example", Name: "lib", Version: "2.0.0"}, MatchStatus: matcher.MatchStatusExact},
	}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemJava, PackageName: "com.example:lib", AdvisoryID: "CVE-1", SeverityScore: 9.8, FixedVersions: []string{"1.0.0", "2.0.0"}},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 2)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count)
	assert.Equal(t, Count{Critical: 1}, perPackage[1].Count)
	assert.Equal(t, Count{Critical: 1}, summary, "the same CVE across versions of a package is counted once in the summary")
}

func TestEnrichMatchesJavaCaseInsensitively(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.Example", "Lib", matcher.MatchStatusExact)}
	advisories := []Advisory{javaAdvisory("CVE-1", 9.8)}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count, "java package should match advisory regardless of case")
}

func TestEnrichMatchesPythonWithNormalization(t *testing.T) {
	results := []matcher.MatchResult{pythonResult("flask-login", matcher.MatchStatusExact)}
	advisories := []Advisory{
		{Ecosystem: matcher.EcosystemPython, PackageName: "Flask_Login", AdvisoryID: "CVE-1", SeverityScore: 7.5, FixedVersions: []string{"1.0.0"}},
	}

	perPackage, _ := Enrich(results, advisories)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{Important: 1}, perPackage[0].Count, "python names should match after PEP 503 normalization")
}

func TestEnrichAggregatesAcrossPackages(t *testing.T) {
	results := []matcher.MatchResult{
		javaResult("com.example", "lib", matcher.MatchStatusExact),
		pythonResult("flask", matcher.MatchStatusExact),
	}
	advisories := []Advisory{
		javaAdvisory("CVE-1", 9.8),
		{Ecosystem: matcher.EcosystemPython, PackageName: "flask", AdvisoryID: "CVE-2", SeverityScore: 7.5, FixedVersions: []string{"1.0.0"}},
		{Ecosystem: matcher.EcosystemPython, PackageName: "flask", AdvisoryID: "CVE-3", SeverityScore: 4.0, FixedVersions: []string{"1.0.0"}},
	}

	perPackage, summary := Enrich(results, advisories)

	require.Len(t, perPackage, 2)
	assert.Equal(t, Count{Critical: 1}, perPackage[0].Count)
	assert.Equal(t, Count{Important: 1, Moderate: 1}, perPackage[1].Count)
	assert.Equal(t, Count{Critical: 1, Important: 1, Moderate: 1}, summary)
}

func TestEnrichPackageWithNoAdvisories(t *testing.T) {
	results := []matcher.MatchResult{javaResult("com.example", "lib", matcher.MatchStatusExact)}

	perPackage, summary := Enrich(results, nil)

	require.Len(t, perPackage, 1)
	assert.Equal(t, Count{}, perPackage[0].Count)
	assert.Nil(t, perPackage[0].Range)
	assert.Equal(t, Count{}, summary)
}

func TestExactMatchKeysGroupsNormalizedKeysByEcosystem(t *testing.T) {
	results := []matcher.MatchResult{
		javaResult("com.Example", "Lib", matcher.MatchStatusExact),
		pythonResult("Flask_Cors", matcher.MatchStatusExact),
	}

	keys := ExactMatchKeys(results)

	assert.Equal(t, []string{"com.example:lib"}, keys[matcher.EcosystemJava], "java keys should be lowercased")
	assert.Equal(t, []string{"flask-cors"}, keys[matcher.EcosystemPython], "python keys should be PEP 503 normalized")
}

func TestExactMatchKeysExcludesNonExactMatches(t *testing.T) {
	results := []matcher.MatchResult{
		javaResult("com.example", "exact", matcher.MatchStatusExact),
		javaResult("com.example", "partial", matcher.MatchStatusPartial),
		javaResult("com.example", "none", matcher.MatchStatusNone),
	}

	keys := ExactMatchKeys(results)

	assert.Equal(t, []string{"com.example:exact"}, keys[matcher.EcosystemJava], "only exact matches contribute keys")
}

func TestExactMatchKeysDedupesAndSorts(t *testing.T) {
	results := []matcher.MatchResult{
		javaResult("com.example", "beta", matcher.MatchStatusExact),
		javaResult("com.example", "alpha", matcher.MatchStatusExact),
		javaResult("com.Example", "Alpha", matcher.MatchStatusExact), // same normalized key as alpha
	}

	keys := ExactMatchKeys(results)

	assert.Equal(t, []string{"com.example:alpha", "com.example:beta"}, keys[matcher.EcosystemJava])
}

func TestExactMatchKeysOmitsUnsupportedEcosystems(t *testing.T) {
	results := []matcher.MatchResult{
		{Package: matcher.Package{Ecosystem: "npm", Name: "left-pad", Version: "1.0.0"}, MatchStatus: matcher.MatchStatusExact},
	}

	keys := ExactMatchKeys(results)

	assert.Empty(t, keys, "ecosystems the catalog cannot match should be omitted")
}
