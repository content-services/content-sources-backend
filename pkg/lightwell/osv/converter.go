// Package osv converts Lightwell advisories into osv.dev-compatible records and
// answers OSV-style version queries against them. It backs the /demo/osvdev mock
// that serves our Lightwell content until it is accepted upstream by osv.dev.
package osv

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
)

// DefaultEcosystem is used for advisories because LightwellAdvisory has no
// ecosystem column. This is a documented simplification of the mock: all records
// are served under a single ecosystem directory in the static export.
const DefaultEcosystem = "Red Hat"

// schemaVersion is the OSV schema version we advertise.
const schemaVersion = "1.6.0"

// Ecosystem returns the OSV ecosystem name for an advisory.
func Ecosystem(_ models.LightwellAdvisory) string {
	return DefaultEcosystem
}

// BuildRecords groups advisories by AdvisoryID and maps each group to a single OSV
// record (one affected[] entry per advisory row, so one OSV id can cover several
// packages). Input order is preserved by advisory id.
func BuildRecords(advisories []models.LightwellAdvisory) []api.OsvVulnerability {
	order := make([]string, 0, len(advisories))
	groups := make(map[string][]models.LightwellAdvisory)
	for _, a := range advisories {
		if _, ok := groups[a.AdvisoryID]; !ok {
			order = append(order, a.AdvisoryID)
		}
		groups[a.AdvisoryID] = append(groups[a.AdvisoryID], a)
	}

	records := make([]api.OsvVulnerability, 0, len(order))
	for _, id := range order {
		records = append(records, buildRecord(id, groups[id]))
	}
	return records
}

func buildRecord(id string, rows []models.LightwellAdvisory) api.OsvVulnerability {
	rec := api.OsvVulnerability{
		SchemaVersion: schemaVersion,
		ID:            id,
	}

	var modified, published time.Time
	seenRefs := map[string]bool{}
	for _, r := range rows {
		if modified.IsZero() || r.UpdatedAt.After(modified) {
			modified = r.UpdatedAt
		}
		if published.IsZero() || r.CreatedAt.Before(published) {
			published = r.CreatedAt
		}
		if rec.Details == "" && r.Details != "" {
			rec.Details = r.Details
		}
		if len(rec.Severity) == 0 {
			if sev := severity(r.Severity); sev != nil {
				rec.Severity = []api.OsvSeverity{*sev}
			}
		}
		rec.Affected = append(rec.Affected, affected(r))
		for _, u := range r.ReferenceURLs {
			if u == "" || seenRefs[u] {
				continue
			}
			seenRefs[u] = true
			rec.References = append(rec.References, api.OsvReference{Type: "ADVISORY", URL: u})
		}
	}

	if !modified.IsZero() {
		rec.Modified = modified.UTC().Format(time.RFC3339)
	}
	if !published.IsZero() {
		rec.Published = published.UTC().Format(time.RFC3339)
	}
	return rec
}

// severity maps a Lightwell severity to an OSV severity entry. Lightwell stores a
// qualitative label (Critical/Important/...) or a CVSS vector; OSV's score field
// expects a CVSS vector, so we only emit an entry for vectors.
func severity(s string) *api.OsvSeverity {
	if strings.HasPrefix(s, "CVSS:") {
		return &api.OsvSeverity{Type: "CVSS_V3", Score: s}
	}
	return nil
}

func affected(a models.LightwellAdvisory) api.OsvAffected {
	events := []api.OsvEvent{{Introduced: "0"}}
	for _, v := range a.FixedVersions {
		if v == "" {
			continue
		}
		events = append(events, api.OsvEvent{Fixed: v})
	}
	return api.OsvAffected{
		Package: api.OsvPackage{Ecosystem: Ecosystem(a), Name: a.PackageName},
		Ranges:  []api.OsvRange{{Type: "ECOSYSTEM", Events: events}},
	}
}

// Matches reports whether a record satisfies an OSV query. commit queries never
// match (we have no GIT ranges). A version query matches when the package name
// (and ecosystem, if supplied) matches an affected entry and the version is not
// yet fixed.
func Matches(rec api.OsvVulnerability, q api.OsvQuery) bool {
	if q.Commit != "" {
		return false
	}
	name, ecosystem := queryPackage(q)
	if name == "" {
		return false
	}
	for _, aff := range rec.Affected {
		if !strings.EqualFold(aff.Package.Name, name) {
			continue
		}
		if ecosystem != "" && !strings.EqualFold(aff.Package.Ecosystem, ecosystem) {
			continue
		}
		if q.Version == "" || versionAffected(q.Version, aff) {
			return true
		}
	}
	return false
}

// queryPackage resolves the package name and ecosystem from a query, preferring
// explicit fields and falling back to a purl.
func queryPackage(q api.OsvQuery) (name, ecosystem string) {
	name = strings.TrimSpace(q.Package.Name)
	ecosystem = strings.TrimSpace(q.Package.Ecosystem)
	if name == "" && q.Package.Purl != "" {
		name, _ = parsePurl(q.Package.Purl)
	}
	return name, ecosystem
}

// parsePurl extracts the package name and version from a Package URL such as
// "pkg:npm/left-pad@1.3.0". It is intentionally lenient (best-effort for the mock).
func parsePurl(purl string) (name, version string) {
	p := strings.TrimPrefix(purl, "pkg:")
	if at := strings.LastIndex(p, "@"); at != -1 {
		version = p[at+1:]
		p = p[:at]
	}
	if slash := strings.LastIndex(p, "/"); slash != -1 {
		p = p[slash+1:]
	}
	return p, version
}

// versionAffected reports whether version is affected given an affected entry:
// affected when it is strictly below the lowest fixed version, or when no fixed
// version is known (the whole package is affected).
func versionAffected(version string, aff api.OsvAffected) bool {
	var fixed []string
	for _, r := range aff.Ranges {
		for _, e := range r.Events {
			if e.Fixed != "" {
				fixed = append(fixed, e.Fixed)
			}
		}
	}
	if len(fixed) == 0 {
		return true
	}
	for _, f := range fixed {
		if compareVersions(version, f) < 0 {
			return true
		}
	}
	return false
}

// compareVersions compares two dotted version strings segment by segment. Numeric
// segments compare numerically; otherwise they compare lexically. Returns -1, 0,
// or 1. This is a best-effort comparator sufficient for the demo mock.
func compareVersions(a, b string) int {
	as := splitVersion(a)
	bs := splitVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		ai, aErr := strconv.Atoi(av)
		bi, bErr := strconv.Atoi(bv)
		if aErr == nil && bErr == nil {
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
			continue
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+' || r == '~' || r == '_'
	})
}

// SortRecords orders records by id for stable output (e.g. the static export).
func SortRecords(recs []api.OsvVulnerability) {
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
}
