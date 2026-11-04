package matcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.True(t, python.Supported)

	java := summaryMap[EcosystemJava]
	assert.Equal(t, 2, java.Total)
	assert.Equal(t, 1, java.ExactMatches)
	assert.Equal(t, 0, java.PartialMatches)
	assert.Equal(t, 1, java.Unmatched)
	assert.True(t, java.Supported)
}

// TestMatchCatalog_Status checks the per-package status for each way a manifest entry can relate to the catalog.
func TestMatchCatalog_Status(t *testing.T) {
	catalog := append(append([]Package{}, pythonCatalog...), javaCatalog...)

	for _, tc := range []struct {
		name string
		pkg  Package
		want string
	}{
		{"python exact", Package{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"}, MatchStatusExact},
		{"python version not in catalog", Package{Ecosystem: EcosystemPython, Name: "flask", Version: "2.0.0"}, MatchStatusPartial},
		{"python without version", Package{Ecosystem: EcosystemPython, Name: "flask"}, MatchStatusPartial},
		{"python unknown name", Package{Ecosystem: EcosystemPython, Name: "custom-lib", Version: "0.1.0"}, MatchStatusNone},
		{"java exact", Package{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "6.1.0"}, MatchStatusExact},
		{"java version not in catalog", Package{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.0"}, MatchStatusPartial},
		{"java unknown name", Package{Ecosystem: EcosystemJava, Namespace: "com.internal", Name: "internal-lib", Version: "1.0.0"}, MatchStatusNone},
		// characterization: rebuilds of a cataloged upstream version are partial today (LWLP-4448).
		{"python rebuild of cataloged version", Package{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3+rhlw.1"}, MatchStatusPartial},
		{"java rebuild of cataloged version", Package{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "6.1.0.rhlw.00003"}, MatchStatusPartial},
		{"python rebuild of uncataloged version", Package{Ecosystem: EcosystemPython, Name: "flask", Version: "2.0.0+rhlw.1"}, MatchStatusPartial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, _ := MatchCatalog(catalog, []Package{tc.pkg}, snapshotAt)

			require.Len(t, results, 1)
			assert.Equal(t, tc.want, results[0].MatchStatus)
			assert.Equal(t, tc.pkg, results[0].Package, "the manifest package must be reported unchanged")
		})
	}
}

func TestMatchCatalog_IncludesAllEcosystems(t *testing.T) {
	manifest := []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: "JavaScript", Name: "express", Version: "4.18.2"},
		{Ecosystem: "Go", Name: "gin", Version: "1.9.1", Namespace: "github.com/gin-gonic"},
		{Ecosystem: "Generic", Name: "example", Version: "1.0.0"},
	}

	results, summary := MatchCatalog(pythonCatalog, manifest, snapshotAt)

	assert.Equal(t, MatchStatusExact, results[0].MatchStatus)
	assert.Equal(t, MatchStatusNone, results[1].MatchStatus)
	assert.Equal(t, MatchStatusNone, results[2].MatchStatus)
	assert.Equal(t, MatchStatusNone, results[3].MatchStatus)

	assert.Equal(t, 4, summary.Total)
	assert.Equal(t, 1, summary.ExactMatches)
	assert.Equal(t, 0, summary.PartialMatches)
	assert.Equal(t, 3, summary.Unmatched)

	summaryMap := map[string]EcosystemSummary{}
	for _, e := range summary.EcosystemCoverageSummary {
		summaryMap[e.Ecosystem] = e
	}

	python := summaryMap[EcosystemPython]
	assert.Equal(t, 1, python.Total)
	assert.Equal(t, 1, python.ExactMatches)
	assert.Equal(t, 0, python.PartialMatches)
	assert.Equal(t, 0, python.Unmatched)
	assert.True(t, python.Supported)

	javascript := summaryMap["JavaScript"]
	assert.Equal(t, 1, javascript.Total)
	assert.Equal(t, 0, javascript.ExactMatches)
	assert.Equal(t, 0, javascript.PartialMatches)
	assert.Equal(t, 1, javascript.Unmatched)
	assert.False(t, javascript.Supported)

	golang := summaryMap["Go"]
	assert.Equal(t, 1, golang.Total)
	assert.Equal(t, 0, golang.ExactMatches)
	assert.Equal(t, 0, golang.PartialMatches)
	assert.Equal(t, 1, golang.Unmatched)
	assert.False(t, golang.Supported)

	generic := summaryMap["Generic"]
	assert.Equal(t, 1, generic.Total)
	assert.Equal(t, 0, generic.ExactMatches)
	assert.Equal(t, 0, generic.PartialMatches)
	assert.Equal(t, 1, generic.Unmatched)
	assert.False(t, generic.Supported)
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

func TestPackageFullName(t *testing.T) {
	for _, tc := range []struct {
		pkg  Package
		want string
	}{
		{Package{Ecosystem: "Go", Namespace: "github.com/pelletier/go-toml", Name: "v2"}, "github.com/pelletier/go-toml/v2"},
		{Package{Ecosystem: "JavaScript", Namespace: "@angular", Name: "core"}, "@angular/core"},
		{Package{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core"}, "org.springframework:spring-core"},
		{Package{Ecosystem: EcosystemPython, Name: "flask"}, "flask"},
	} {
		assert.Equal(t, tc.want, tc.pkg.FullName())
	}
}

func TestMatchCatalog_NamespacesRemainDistinct(t *testing.T) {
	for _, ecosystem := range []string{"Go", "JavaScript"} {
		t.Run(ecosystem, func(t *testing.T) {
			catalog := []Package{{Ecosystem: ecosystem, Namespace: "first", Name: "v2", Version: "2.0.0"}}
			manifest := []Package{
				catalog[0],
				{Ecosystem: ecosystem, Namespace: "second", Name: "v2", Version: "2.0.0"},
			}
			results, _ := MatchCatalog(catalog, manifest, snapshotAt)
			assert.Equal(t, MatchStatusExact, results[0].MatchStatus)
			assert.Equal(t, MatchStatusNone, results[1].MatchStatus)
		})
	}
}
