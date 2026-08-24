package matcher

import (
	"strings"
	"time"
)

const (
	MatchStatusExact   = "exact"
	MatchStatusPartial = "partial"
	MatchStatusNone    = "none"
)

const (
	EcosystemJava   = "Java"
	EcosystemPython = "Python"
)

type Package struct {
	Ecosystem string
	Name      string
	Version   string
	Namespace string
}

type MatchResult struct {
	Package
	MatchStatus string
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
	Total          int
	ExactMatches   int
	PartialMatches int
	Unmatched      int
}

// MatchCatalog compares manifest packages against a catalog and returns per-package match results and an aggregate summary.
func MatchCatalog(catalog, parsedPackages []Package, snapshotAt time.Time) ([]MatchResult, MatchSummary) {
	catalogedNames, catalogedNameVersions := buildIndex(catalog)

	results := make([]MatchResult, len(parsedPackages))
	ecosystemSummaries := map[string]*EcosystemSummary{}

	for i, pkg := range parsedPackages {
		status := matchPackage(pkg, catalogedNames, catalogedNameVersions)
		results[i] = MatchResult{
			Package:     pkg,
			MatchStatus: status,
		}

		entry, exists := ecosystemSummaries[pkg.Ecosystem]
		if !exists {
			entry = &EcosystemSummary{Ecosystem: pkg.Ecosystem}
			ecosystemSummaries[pkg.Ecosystem] = entry
		}
		entry.Total++
		switch status {
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

func matchPackage(pkg Package, catalogedNames, catalogedNameVersions map[string]struct{}) string {
	nameKey := normalizeKey(pkg)

	if pkg.Version != "" {
		versionKey := nameKey + ":" + pkg.Version
		if _, found := catalogedNameVersions[versionKey]; found {
			return MatchStatusExact
		}
	}

	if _, found := catalogedNames[nameKey]; found {
		return MatchStatusPartial
	}

	return MatchStatusNone
}

func buildIndex(catalog []Package) (catalogedNames, catalogedNameVersions map[string]struct{}) {
	catalogedNames = make(map[string]struct{}, len(catalog))
	catalogedNameVersions = make(map[string]struct{}, len(catalog))

	for _, pkg := range catalog {
		key := normalizeKey(pkg)
		catalogedNames[key] = struct{}{}
		if pkg.Version != "" {
			catalogedNameVersions[key+":"+pkg.Version] = struct{}{}
		}
	}

	return catalogedNames, catalogedNameVersions
}

func normalizeKey(pkg Package) string {
	switch pkg.Ecosystem {
	case EcosystemPython:
		return normalizePythonName(pkg.Name)
	case EcosystemJava:
		return strings.ToLower(pkg.Namespace) + ":" + strings.ToLower(pkg.Name)
	default:
		return strings.ToLower(pkg.Ecosystem) + ":" + strings.ToLower(pkg.Name)
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
