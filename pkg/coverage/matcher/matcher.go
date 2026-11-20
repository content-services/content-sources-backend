package matcher

import (
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/rs/zerolog/log"
)

const (
	MatchStatusExact   = "exact"
	MatchStatusPartial = "partial"
	MatchStatusNone    = "none"
)

const (
	EcosystemJavaScript = "JavaScript"
	EcosystemJava       = "Java"
	EcosystemPython     = "Python"
)

type Package struct {
	Ecosystem string
	Name      string
	Version   string
	Namespace string
	// PublishedAt is set on catalog entries. It orders versions that are not numeric.
	// Manifest packages leave it empty.
	PublishedAt string
}

// FullName preserves the namespace and name using the ecosystem's separator.
func (p Package) FullName() string {
	if p.Namespace == "" {
		return p.Name
	}
	if p.Ecosystem == EcosystemJava {
		return p.Namespace + ":" + p.Name
	}
	return p.Namespace + "/" + p.Name
}

type MatchResult struct {
	Package
	MatchStatus          string
	LightwellVersions    []string
	NearestHigherVersion string
	MatchSegment         string
}

type MatchSummary struct {
	Total                    int
	ExactMatches             int
	PartialMatches           int
	Unmatched                int
	EcosystemCoverageSummary []EcosystemSummary
	CatalogSnapshotAt        time.Time
}

type EcosystemSummary struct {
	Ecosystem      string
	Supported      bool
	Total          int
	ExactMatches   int
	PartialMatches int
	Unmatched      int
}

// IsSupportedEcosystem reports whether the Lightwell catalog can match packages for the given ecosystem.
func IsSupportedEcosystem(ecosystem string) bool {
	switch ecosystem {
	case EcosystemJava, EcosystemPython, EcosystemJavaScript:
		return true
	default:
		return false
	}
}

// MatchCatalog compares manifest packages against a catalog and returns per-package match results and an aggregate summary.
func MatchCatalog(catalog, parsedPackages []Package, snapshotAt time.Time) ([]MatchResult, MatchSummary) {
	index := newCatalogIndex(catalog)

	results := make([]MatchResult, len(parsedPackages))
	ecosystemSummaries := map[string]*EcosystemSummary{}

	for i, pkg := range parsedPackages {
		result := matchPackage(pkg, index)
		results[i] = result

		entry, exists := ecosystemSummaries[pkg.Ecosystem]
		if !exists {
			entry = &EcosystemSummary{
				Ecosystem: pkg.Ecosystem,
				Supported: IsSupportedEcosystem(pkg.Ecosystem),
			}
			ecosystemSummaries[pkg.Ecosystem] = entry
		}
		entry.Total++
		switch result.MatchStatus {
		case MatchStatusExact:
			entry.ExactMatches++
		case MatchStatusPartial:
			entry.PartialMatches++
		case MatchStatusNone:
			entry.Unmatched++
		}
	}

	summary := MatchSummary{
		CatalogSnapshotAt:        snapshotAt,
		EcosystemCoverageSummary: []EcosystemSummary{},
	}
	for _, entry := range ecosystemSummaries {
		summary.Total += entry.Total
		summary.ExactMatches += entry.ExactMatches
		summary.PartialMatches += entry.PartialMatches
		summary.Unmatched += entry.Unmatched
		summary.EcosystemCoverageSummary = append(summary.EcosystemCoverageSummary, *entry)
	}

	return results, summary
}

// matchPackage classifies one manifest package. The catalog stores upstream versions only,
// so a Lightwell rebuild version (for example "3.0.1+rhlw.1") is compared by its upstream part.
// An upstream version with more than four dot components is a partial match with match segment patch.
// A trailing word such as Final is a tag and is compared by the numeric prefix in front of it.
func matchPackage(pkg Package, index catalogIndex) MatchResult {
	nameKey := NormalizeKey(pkg)
	catalogPkg := index.packages[nameKey]
	userUpstream, _ := rhlw.SplitVersion(pkg.Version)
	userTooLong := versionHasTooManyComponents(userUpstream)
	if userTooLong {
		warnTooManyComponents(pkg, userUpstream)
	}

	result := MatchResult{
		Package:      pkg,
		MatchStatus:  MatchStatusNone,
		MatchSegment: MatchSegmentNone,
	}
	if catalogPkg == nil {
		result.LightwellVersions = []string{}
		return result
	}

	result.LightwellVersions = catalogPkg.versionList()
	if userUpstream != "" && catalogPkg.hasVersion(userUpstream) && !userTooLong {
		result.MatchStatus = MatchStatusExact
		return result
	}

	result.MatchStatus = MatchStatusPartial
	if userUpstream == "" {
		return result
	}

	nearest, segment, nearestTooLong := catalogPkg.nearestHigher(userUpstream)
	result.NearestHigherVersion = nearest
	result.MatchSegment = segment
	if nearestTooLong {
		warnTooManyComponents(pkg, nearest)
		result.MatchSegment = MatchSegmentPatch
	}
	if userTooLong {
		result.MatchSegment = MatchSegmentPatch
	}
	return result
}

func warnTooManyComponents(pkg Package, version string) {
	log.Warn().
		Str("ecosystem", pkg.Ecosystem).
		Str("package", pkg.FullName()).
		Str("version", version).
		Msg("upstream version has more than 4 components")
}

// NormalizeKey returns the canonical lookup key for a package: PEP 503-normalized
// name for Python, lowercased "namespace:name" for Java.
func NormalizeKey(pkg Package) string {
	switch pkg.Ecosystem {
	case EcosystemPython:
		return normalizePythonName(pkg.Name)
	case EcosystemJava:
		return strings.ToLower(pkg.Namespace) + ":" + strings.ToLower(pkg.Name)
	default:
		return strings.ToLower(pkg.Ecosystem) + ":" + strings.ToLower(pkg.FullName())
	}
}

// normalizePythonName applies PEP 503 normalization: lowercase, replace [-_.] with -.
func normalizePythonName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	separatorWritten := false
	for _, ch := range strings.ToLower(name) {
		if ch == '-' || ch == '_' || ch == '.' {
			if !separatorWritten {
				b.WriteByte('-')
				separatorWritten = true
			}
		} else {
			b.WriteRune(ch)
			separatorWritten = false
		}
	}
	return b.String()
}
