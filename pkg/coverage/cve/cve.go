package cve

import (
	"sort"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
)

// CVSS base-score thresholds used to bucket a CVE into a severity level.
// A score below lowThreshold (e.g. 0.0 / unscored) is excluded entirely.
// Severity labels mirror the rest of the service (critical/important/moderate/low).
const (
	criticalThreshold  = 9.0
	importantThreshold = 7.0
	moderateThreshold  = 4.0
	lowThreshold       = 0.1
)

// Advisory is a CVE fixed in a remediated repository for a package.
type Advisory struct {
	Ecosystem     string   // matcher.EcosystemJava or matcher.EcosystemPython
	PackageName   string   // Java: "groupId:artifactId"; Python: PyPI project name
	AdvisoryID    string   // CVE / advisory identifier (used to de-duplicate)
	SeverityScore float32  // CVSS base score
	FixedVersions []string // package versions in which the advisory is fixed
}

// Count is a per-severity count of CVEs.
type Count struct {
	Critical  int
	Important int
	Moderate  int
	Low       int
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

// Enrich computes per-package CVE data for exact-matched packages and an aggregate
// summary. CVE data is version-scoped: an advisory only applies to a package when the
// package's version is one of the advisory's fixed versions. Only exact matches (name +
// version) are enriched, since a version-scoped advisory cannot be tied to a package
// whose version was not matched in the catalog.
//
// The returned slice is aligned 1:1 with results; packages that are not exact matches,
// or that have no matching advisories, get a zero Count and a nil Range. Within a package
// CVEs are de-duplicated by AdvisoryID, and the summary de-duplicates by (package,
// AdvisoryID) so a CVE spanning multiple versions of the same package is counted once.
// CVEs scoring below lowThreshold are excluded from both the counts and the range.
func Enrich(results []matcher.MatchResult, advisories []Advisory) ([]PackageCVE, Count) {
	index := buildAdvisoryIndex(advisories)

	perPackage := make([]PackageCVE, len(results))
	summaryScores := make(map[string]float32)
	for i, result := range results {
		if result.MatchStatus != matcher.MatchStatusExact {
			continue
		}
		key := matcher.NormalizeKey(result.Package)
		scores := index[key][result.Version]
		if len(scores) == 0 {
			continue
		}
		perPackage[i] = computePackageCVE(scores)
		for advisoryID, score := range scores {
			summaryScores[key+"\x00"+advisoryID] = score
		}
	}

	var summary Count
	for _, score := range summaryScores {
		bucketScore(&summary, score)
	}
	return perPackage, summary
}

// ExactMatchKeys returns, grouped by ecosystem, the normalized package keys of the
// exact-matched results. These are the only packages Enrich can ever enrich (it skips
// non-exact matches and looks up advisories by normalized key), so a caller can use
// this set to fetch just the advisories that could possibly match instead of the whole
// catalog. Keys are deduplicated and sorted for deterministic output. Ecosystems the
// catalog cannot match are omitted, since advisories only exist for supported ones.
func ExactMatchKeys(results []matcher.MatchResult) map[string][]string {
	sets := make(map[string]map[string]struct{})
	for _, result := range results {
		if result.MatchStatus != matcher.MatchStatusExact {
			continue
		}
		if !matcher.IsSupportedEcosystem(result.Ecosystem) {
			continue
		}
		key := matcher.NormalizeKey(result.Package)
		if key == "" {
			continue
		}
		set, ok := sets[result.Ecosystem]
		if !ok {
			set = make(map[string]struct{})
			sets[result.Ecosystem] = set
		}
		set[key] = struct{}{}
	}

	out := make(map[string][]string, len(sets))
	for ecosystem, set := range sets {
		keys := make([]string, 0, len(set))
		for key := range set {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out[ecosystem] = keys
	}
	return out
}

// buildAdvisoryIndex maps a normalized package key to its fixed versions, and each fixed
// version to the set of distinct advisory IDs (and their scores) fixed in that version.
func buildAdvisoryIndex(advisories []Advisory) map[string]map[string]map[string]float32 {
	index := make(map[string]map[string]map[string]float32)
	for _, advisory := range advisories {
		key := advisoryKey(advisory.Ecosystem, advisory.PackageName)
		if key == "" {
			continue
		}
		byVersion, ok := index[key]
		if !ok {
			byVersion = make(map[string]map[string]float32)
			index[key] = byVersion
		}
		for _, version := range advisory.FixedVersions {
			if version == "" {
				continue
			}
			byID, ok := byVersion[version]
			if !ok {
				byID = make(map[string]float32)
				byVersion[version] = byID
			}
			byID[advisory.AdvisoryID] = advisory.SeverityScore
		}
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
		if !bucketScore(&count, score) {
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

// bucketScore increments the matching severity bucket for a CVSS score and reports
// whether the score was counted (false when it falls below the low threshold).
func bucketScore(count *Count, score float32) bool {
	switch {
	case score >= criticalThreshold:
		count.Critical++
	case score >= importantThreshold:
		count.Important++
	case score >= moderateThreshold:
		count.Moderate++
	case score >= lowThreshold:
		count.Low++
	default:
		return false
	}
	return true
}
