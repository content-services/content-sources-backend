package matcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var snapshotAt = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

var pythonCatalog = []Package{
	{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
	{Ecosystem: EcosystemPython, Name: "flask", Version: "2.3.0"},
	{Ecosystem: EcosystemPython, Name: "numpy", Version: "1.26.0"},
	{Ecosystem: EcosystemPython, Name: "ruamel-yaml", Version: "0.18.6"},
}

var javaCatalog = []Package{
	{Ecosystem: EcosystemJava, Name: "spring-core", Version: "6.1.0", Namespace: "org.springframework"},
}

func TestMatchCatalog_EcosystemSummary(t *testing.T) {
	catalog := append(pythonCatalog, javaCatalog...)
	manifest := []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemPython, Name: "numpy", Version: "2.0.0"},
		{Ecosystem: EcosystemPython, Name: "custom-lib", Version: "0.1.0"},
		{Ecosystem: EcosystemJava, Name: "spring-core", Version: "6.1.0", Namespace: "org.springframework"},
		{Ecosystem: EcosystemJava, Name: "guava", Version: "32.0", Namespace: "com.google.guava"},
	}

	_, summary := MatchCatalog(catalog, manifest, snapshotAt)

	assert.Equal(t, 5, summary.Total)
	assert.Equal(t, 2, summary.ExactMatches)
	assert.Equal(t, 1, summary.PartialMatches)
	assert.Equal(t, 2, summary.Unmatched)
	assert.Equal(t, snapshotAt, summary.CatalogSnapshotAt)

	assert.Len(t, summary.EcosystemCoverageSummary, 2)
	summaryMap := map[string]EcosystemSummary{}
	for _, e := range summary.EcosystemCoverageSummary {
		summaryMap[e.Ecosystem] = e
	}

	python := summaryMap[EcosystemPython]
	assert.Equal(t, 3, python.Total)
	assert.Equal(t, 1, python.ExactMatches)
	assert.Equal(t, 1, python.PartialMatches)
	assert.Equal(t, 1, python.Unmatched)

	java := summaryMap[EcosystemJava]
	assert.Equal(t, 2, java.Total)
	assert.Equal(t, 1, java.ExactMatches)
	assert.Equal(t, 0, java.PartialMatches)
	assert.Equal(t, 1, java.Unmatched)
}

func TestMatchCatalog_ExactMatch(t *testing.T) {
	manifest := []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
	}

	results, summary := MatchCatalog(pythonCatalog, manifest, snapshotAt)

	assert.Len(t, results, 1)
	assert.Equal(t, MatchStatusExact, results[0].MatchStatus)
	assert.Equal(t, "flask", results[0].Name)
	assert.Equal(t, "3.0.3", results[0].Version)
	assert.Equal(t, 1, summary.ExactMatches)
	assert.Equal(t, 0, summary.PartialMatches)
	assert.Equal(t, 0, summary.Unmatched)
	assert.Equal(t, 1, summary.Total)
}

func TestMatchCatalog_PartialMatch_VersionNotFound(t *testing.T) {
	manifest := []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "2.0.0"},
	}

	results, summary := MatchCatalog(pythonCatalog, manifest, snapshotAt)

	assert.Len(t, results, 1)
	assert.Equal(t, MatchStatusPartial, results[0].MatchStatus)
	assert.Equal(t, "flask", results[0].Name)
	assert.Equal(t, 1, summary.PartialMatches)
}

func TestMatchCatalog_PartialMatch_NoVersion(t *testing.T) {
	manifest := []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: ""},
	}

	results, summary := MatchCatalog(pythonCatalog, manifest, snapshotAt)

	assert.Len(t, results, 1)
	assert.Equal(t, MatchStatusPartial, results[0].MatchStatus)
	assert.Equal(t, "flask", results[0].Name)
	assert.Empty(t, results[0].Version)
	assert.Equal(t, 1, summary.PartialMatches)
}

func TestMatchCatalog_JavaMatch(t *testing.T) {
	manifest := []Package{
		{Ecosystem: EcosystemJava, Name: "spring-core", Version: "6.1.0", Namespace: "org.springframework"},
		{Ecosystem: EcosystemJava, Name: "spring-core", Version: "5.3.0", Namespace: "org.springframework"},
		{Ecosystem: EcosystemJava, Name: "internal-lib", Version: "1.0.0", Namespace: "com.internal"},
	}

	results, _ := MatchCatalog(javaCatalog, manifest, snapshotAt)

	assert.Equal(t, MatchStatusExact, results[0].MatchStatus)
	assert.Equal(t, "spring-core", results[0].Name)
	assert.Equal(t, "6.1.0", results[0].Version)
	assert.Equal(t, MatchStatusPartial, results[1].MatchStatus)
	assert.Equal(t, "spring-core", results[1].Name)
	assert.Equal(t, MatchStatusNone, results[2].MatchStatus)
}

func TestNormalizePythonName(t *testing.T) {
	assert.Equal(t, "flask", normalizePythonName("Flask"))
	assert.Equal(t, "ruamel-yaml", normalizePythonName("ruamel.yaml"))
	assert.Equal(t, "my-package", normalizePythonName("my_package"))
	assert.Equal(t, "some-lib", normalizePythonName("Some.Lib"))
	assert.Equal(t, "foo-bar", normalizePythonName("foo__bar"))
	assert.Equal(t, "foo-bar", normalizePythonName("foo..bar"))
	assert.Equal(t, "foo-bar", normalizePythonName("foo-_bar"))
	assert.Equal(t, "foo-bar", normalizePythonName("foo--bar"))
}
