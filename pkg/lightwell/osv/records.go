// Package osv serves osv.dev-compatible records for the /demo/osvdev mock. The
// records are curated static OSV JSON files (see the data/ directory), embedded in
// the binary and served byte-for-byte so the demo exposes our Lightwell content
// with full fidelity until it is accepted upstream by osv.dev.
package osv

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
)

//go:embed data/*.json
var dataFS embed.FS

// Record is a single embedded OSV record. Raw holds the original file bytes,
// served verbatim to preserve every field of the source document. The remaining
// fields are parsed once, at load time, only to answer version/ecosystem queries.
type Record struct {
	ID         string
	Modified   string
	Ecosystems []string
	Raw        json.RawMessage

	affected []recordAffected
}

// recordAffected is the subset of an OSV affected[] entry needed for matching.
type recordAffected struct {
	Ecosystem string
	Name      string
	Purl      string
	Versions  []string
	Fixed     []string
}

// parsedRecord mirrors the OSV fields we read from each file for indexing/matching.
type parsedRecord struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
	Affected []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
			Purl      string `json:"purl"`
		} `json:"package"`
		Ranges []struct {
			Events []struct {
				Fixed string `json:"fixed"`
			} `json:"events"`
		} `json:"ranges"`
		Versions []string `json:"versions"`
	} `json:"affected"`
}

// records is the immutable, sorted set of embedded records, loaded once at init.
var records = mustLoad()

func mustLoad() []Record {
	recs, err := load()
	if err != nil {
		panic(fmt.Sprintf("loading embedded osv records: %v", err))
	}
	return recs
}

func load() ([]Record, error) {
	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return nil, err
	}
	recs := make([]Record, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := dataFS.ReadFile("data/" + e.Name())
		if err != nil {
			return nil, err
		}
		var p parsedRecord
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", e.Name(), err)
		}
		if p.ID == "" {
			return nil, fmt.Errorf("%s: record has no id", e.Name())
		}

		rec := Record{ID: p.ID, Modified: p.Modified, Raw: json.RawMessage(raw)}
		ecosystems := map[string]bool{}
		for _, a := range p.Affected {
			ra := recordAffected{
				Ecosystem: a.Package.Ecosystem,
				Name:      a.Package.Name,
				Purl:      a.Package.Purl,
				Versions:  a.Versions,
			}
			for _, rng := range a.Ranges {
				for _, ev := range rng.Events {
					if ev.Fixed != "" {
						ra.Fixed = append(ra.Fixed, ev.Fixed)
					}
				}
			}
			rec.affected = append(rec.affected, ra)
			if a.Package.Ecosystem != "" {
				ecosystems[a.Package.Ecosystem] = true
			}
		}
		for eco := range ecosystems {
			rec.Ecosystems = append(rec.Ecosystems, eco)
		}
		sort.Strings(rec.Ecosystems)
		recs = append(recs, rec)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	return recs, nil
}

// Records returns all embedded records, sorted by id.
func Records() []Record {
	return records
}

// RecordByID returns the record with the given id, or false if none matches.
func RecordByID(id string) (Record, bool) {
	for _, r := range records {
		if r.ID == id {
			return r, true
		}
	}
	return Record{}, false
}

// Ecosystems returns the sorted, distinct ecosystem names across all records.
func Ecosystems() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range records {
		for _, e := range r.Ecosystems {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	sort.Strings(out)
	return out
}

// InEcosystem reports whether the record has any affected package in ecosystem
// (case-insensitive).
func (r Record) InEcosystem(ecosystem string) bool {
	for _, e := range r.Ecosystems {
		if strings.EqualFold(e, ecosystem) {
			return true
		}
	}
	return false
}

// Matches reports whether a record satisfies an OSV query. commit queries never
// match (our records carry only ECOSYSTEM ranges). A version query matches when
// the package name (and ecosystem, if supplied) matches an affected entry and the
// version is affected (not yet fixed, or explicitly listed).
func Matches(r Record, q api.OsvQuery) bool {
	if q.Commit != "" {
		return false
	}
	name, ecosystem := queryPackage(q)
	if name == "" {
		return false
	}
	for _, aff := range r.affected {
		if !strings.EqualFold(aff.Name, name) {
			continue
		}
		if ecosystem != "" && !strings.EqualFold(aff.Ecosystem, ecosystem) {
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
// "pkg:maven/org.springframework/spring-core@5.3.18". It is intentionally lenient
// (best-effort for the mock) and reconstructs Maven's group:artifact name form.
func parsePurl(purl string) (name, version string) {
	p := strings.TrimPrefix(purl, "pkg:")
	if at := strings.LastIndex(p, "@"); at != -1 {
		version = p[at+1:]
		p = p[:at]
	}
	if slash := strings.Index(p, "/"); slash != -1 {
		typ := p[:slash]
		coord := p[slash+1:]
		// Maven purls encode group/artifact; OSV names them "group:artifact".
		if strings.EqualFold(typ, "maven") {
			if s := strings.LastIndex(coord, "/"); s != -1 {
				return coord[:s] + ":" + coord[s+1:], version
			}
		}
		if s := strings.LastIndex(coord, "/"); s != -1 {
			return coord[s+1:], version
		}
		return coord, version
	}
	return p, version
}

// versionAffected reports whether version is affected given an affected entry:
// affected when it is explicitly listed, strictly below the lowest fixed version,
// or when no fixed version is known (the whole package is affected).
func versionAffected(version string, aff recordAffected) bool {
	for _, v := range aff.Versions {
		if v == version {
			return true
		}
	}
	if len(aff.Fixed) == 0 {
		return true
	}
	for _, f := range aff.Fixed {
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
