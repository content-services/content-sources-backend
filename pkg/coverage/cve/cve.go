package cve

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
)

// CVSS base-score thresholds used to bucket a CVE into a severity level.
// A score below lowThreshold (e.g. 0.0 / unscored) is excluded entirely.
const (
	criticalThreshold = 9.0
	highThreshold     = 7.0
	mediumThreshold   = 4.0
	lowThreshold      = 0.1
)

// Advisory is a CVE fixed in a remediated repository for a package.
type Advisory struct {
	Ecosystem     string  // matcher.EcosystemJava or matcher.EcosystemPython
	PackageName   string  // Java: "groupId:artifactId"; Python: PyPI project name
	AdvisoryID    string  // CVE / advisory identifier (used to de-duplicate)
	SeverityScore float32 // CVSS base score
}

// Count is a per-severity count of CVEs.
type Count struct {
	Critical int
	High     int
	Medium   int
	Low      int
}

// Range is the min/max CVSS severity score across a set of CVEs.
type Range struct {
	Low  float32
	High float32
}

// PackageCVE holds the CVE data computed for a single coverage report package.
type PackageCVE struct {
	Count Count
	Range *Range // nil when the package has no counted CVEs
}

// Enrich computes per-package CVE data for matched (exact or partial) packages and an
// aggregate summary. The returned slice is aligned 1:1 with results; unmatched packages
// and packages with no CVEs get a zero Count and a nil Range. CVEs are de-duplicated by
// AdvisoryID per package and bucketed by CVSS score; CVEs scoring below lowThreshold are
// excluded from both the counts and the range.
func Enrich(results []matcher.MatchResult, advisories []Advisory) ([]PackageCVE, Count) {
	index := buildAdvisoryIndex(advisories)

	perPackage := make([]PackageCVE, len(results))
	var summary Count
	for i, result := range results {
		if result.MatchStatus == matcher.MatchStatusNone {
			continue
		}
		scores := index[matcher.NormalizeKey(result.Package)]
		if len(scores) == 0 {
			continue
		}
		pkgCVE := computePackageCVE(scores)
		perPackage[i] = pkgCVE
		summary.Critical += pkgCVE.Count.Critical
		summary.High += pkgCVE.Count.High
		summary.Medium += pkgCVE.Count.Medium
		summary.Low += pkgCVE.Count.Low
	}
	return perPackage, summary
}

// buildAdvisoryIndex maps a normalized package key to the set of distinct advisory
// IDs (and their scores) that apply to it.
func buildAdvisoryIndex(advisories []Advisory) map[string]map[string]float32 {
	index := make(map[string]map[string]float32)
	for _, advisory := range advisories {
		key := advisoryKey(advisory.Ecosystem, advisory.PackageName)
		if key == "" {
			continue
		}
		byID, ok := index[key]
		if !ok {
			byID = make(map[string]float32)
			index[key] = byID
		}
		byID[advisory.AdvisoryID] = advisory.SeverityScore
	}
	return index
}

// advisoryKey normalizes an advisory's package name so it matches matcher.NormalizeKey
// for the corresponding coverage package. For Java the advisory package name is already
// "groupId:artifactId", which equals the matcher's Java key once lowercased.
func advisoryKey(ecosystem, packageName string) string {
	switch ecosystem {
	case matcher.EcosystemPython:
		return matcher.NormalizeKey(matcher.Package{Ecosystem: matcher.EcosystemPython, Name: packageName})
	case matcher.EcosystemJava:
		return strings.ToLower(packageName)
	default:
		return strings.ToLower(packageName)
	}
}

func computePackageCVE(scores map[string]float32) PackageCVE {
	var count Count
	var rng *Range
	for _, score := range scores {
		switch {
		case score >= criticalThreshold:
			count.Critical++
		case score >= highThreshold:
			count.High++
		case score >= mediumThreshold:
			count.Medium++
		case score >= lowThreshold:
			count.Low++
		default:
			// Unscored / below the low threshold: excluded from counts and range.
			continue
		}
		switch {
		case rng == nil:
			rng = &Range{Low: score, High: score}
		case score < rng.Low:
			rng.Low = score
		case score > rng.High:
			rng.High = score
		}
	}
	return PackageCVE{Count: count, Range: rng}
}
