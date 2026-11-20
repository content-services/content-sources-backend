package matcher

import (
	"sort"
	"strconv"
	"strings"
)

const (
	MatchSegmentMajor = "major"
	MatchSegmentMinor = "minor"
	MatchSegmentPatch = "patch"
	MatchSegmentNone  = "none"
)

// maxVersionComponents is major.minor.patch.tag. A longer upstream version is unexpected.
const maxVersionComponents = 4

type catalogVersion struct {
	version     string
	publishedAt string
	parts       []int
	tag         string
	numeric     bool
	tooLong     bool
}

type catalogPackage struct {
	versions map[string]catalogVersion
}

type catalogIndex struct {
	packages map[string]*catalogPackage
}

func newCatalogIndex(catalog []Package) catalogIndex {
	idx := catalogIndex{packages: make(map[string]*catalogPackage, len(catalog))}
	for _, pkg := range catalog {
		key := NormalizeKey(pkg)
		entry := idx.packages[key]
		if entry == nil {
			entry = &catalogPackage{versions: map[string]catalogVersion{}}
			idx.packages[key] = entry
		}
		if pkg.Version == "" {
			continue
		}
		parsed := parseVersion(pkg.Version)
		if existing, ok := entry.versions[pkg.Version]; ok {
			existing.publishedAt = earlierPublishedAt(existing.publishedAt, pkg.PublishedAt)
			entry.versions[pkg.Version] = existing
			continue
		}
		entry.versions[pkg.Version] = catalogVersion{
			version:     pkg.Version,
			publishedAt: pkg.PublishedAt,
			parts:       parsed.parts,
			tag:         parsed.tag,
			numeric:     parsed.ok,
			tooLong:     parsed.tooLong,
		}
	}
	return idx
}

func (p *catalogPackage) hasVersion(version string) bool {
	if p == nil || version == "" {
		return false
	}
	_, ok := p.versions[version]
	return ok
}

// versionList returns distinct upstream versions, numeric versions first in numeric order.
func (p *catalogPackage) versionList() []string {
	if p == nil || len(p.versions) == 0 {
		return []string{}
	}
	list := make([]catalogVersion, 0, len(p.versions))
	for _, version := range p.versions {
		list = append(list, version)
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.numeric != b.numeric {
			return a.numeric
		}
		if a.numeric {
			if cmp := compareNumeric(a.parts, b.parts); cmp != 0 {
				return cmp < 0
			}
		}
		return a.version < b.version
	})
	out := make([]string, len(list))
	for i, version := range list {
		out[i] = version.version
	}
	return out
}

// nearestHigher picks the catalog version to report for a partial match.
// A trailing tag such as Final stays on its numeric prefix. The same prefix with a
// different tag is nearest, and the latest published_at wins. Otherwise the smallest
// strictly higher numeric prefix wins. A version that does not parse falls back to the
// non-numeric catalog version with the earliest published_at.
func (p *catalogPackage) nearestHigher(userUpstream string) (version string, segment string, tooManyComponents bool) {
	if p == nil {
		return "", MatchSegmentNone, false
	}
	parsed := parseVersion(userUpstream)
	if parsed.ok && parsed.tag != "" {
		if best, ok := p.latestDifferentTag(parsed.parts, parsed.tag); ok {
			return best.version, MatchSegmentPatch, best.tooLong
		}
	}
	if parsed.ok {
		if best, ok := p.smallestHigher(parsed.parts); ok {
			return best.version, matchSegment(parsed.parts, best.parts), best.tooLong
		}
	}
	if best, ok := p.earliestNonNumeric(); ok {
		return best.version, MatchSegmentNone, false
	}
	return "", MatchSegmentNone, false
}

// latestDifferentTag returns the catalog version with the same numeric prefix, a different
// tag, and the latest published_at. An empty published_at loses to a real timestamp.
func (p *catalogPackage) latestDifferentTag(userParts []int, userTag string) (catalogVersion, bool) {
	var best catalogVersion
	found := false
	for _, version := range p.versions {
		if !version.numeric || version.tag == "" || version.tag == userTag || compareNumeric(version.parts, userParts) != 0 {
			continue
		}
		if !found || publishedLater(version, best) {
			best = version
			found = true
		}
	}
	return best, found
}

func (p *catalogPackage) smallestHigher(userParts []int) (catalogVersion, bool) {
	var best catalogVersion
	found := false
	for _, version := range p.versions {
		if !version.numeric || compareNumeric(version.parts, userParts) <= 0 {
			continue
		}
		if !found || preferHigher(version, best) {
			best = version
			found = true
		}
	}
	return best, found
}

// preferHigher reports whether candidate is a closer higher version than current.
// A smaller numeric prefix wins. The same prefix with tags uses the latest published_at.
func preferHigher(candidate, current catalogVersion) bool {
	cmp := compareNumeric(candidate.parts, current.parts)
	if cmp != 0 {
		return cmp < 0
	}
	if candidate.tag != "" && current.tag != "" {
		return publishedLater(candidate, current)
	}
	return candidate.version < current.version
}

func (p *catalogPackage) earliestNonNumeric() (catalogVersion, bool) {
	var best catalogVersion
	found := false
	for _, version := range p.versions {
		if version.numeric {
			continue
		}
		if !found || publishedBefore(version, best) {
			best = version
			found = true
		}
	}
	return best, found
}

func versionHasTooManyComponents(version string) bool {
	parsed := parseVersion(version)
	return parsed.ok && parsed.tooLong
}

// matchSegment names the deepest component that still agrees with the higher version.
// A difference in the 4th component, or past it, is patch.
func matchSegment(userParts, higherParts []int) string {
	n := len(userParts)
	if len(higherParts) > n {
		n = len(higherParts)
	}
	firstDiff := -1
	for i := 0; i < n; i++ {
		userComponent := 0
		if i < len(userParts) {
			userComponent = userParts[i]
		}
		higherComponent := 0
		if i < len(higherParts) {
			higherComponent = higherParts[i]
		}
		if userComponent != higherComponent {
			firstDiff = i
			break
		}
	}
	switch firstDiff {
	case 1:
		return MatchSegmentMajor
	case 2:
		return MatchSegmentMinor
	case -1, 0:
		return MatchSegmentNone
	default:
		return MatchSegmentPatch
	}
}

type parsedVersion struct {
	parts   []int
	tag     string
	ok      bool
	tooLong bool
}

// parseVersion splits an upstream version into a numeric prefix and an optional trailing tag.
// The tag is one dot component of letters with optional trailing digits, such as Final or post1.
// A version with more than four dot components is too long.
func parseVersion(version string) parsedVersion {
	if version == "" {
		return parsedVersion{}
	}
	raw := strings.Split(version, ".")
	end := len(raw)
	tag := ""
	if len(raw) > 1 && isTag(raw[len(raw)-1]) {
		tag = raw[len(raw)-1]
		end--
	}
	if end == 0 {
		return parsedVersion{}
	}
	parts := make([]int, end)
	for i := 0; i < end; i++ {
		if raw[i] == "" || !digitsOnly(raw[i]) {
			return parsedVersion{}
		}
		n, err := strconv.Atoi(raw[i])
		if err != nil {
			return parsedVersion{}
		}
		parts[i] = n
	}
	return parsedVersion{parts: parts, tag: tag, ok: true, tooLong: len(raw) > maxVersionComponents}
}

func isTag(component string) bool {
	if component == "" {
		return false
	}
	for i, ch := range component {
		if i == 0 {
			if !isLetter(ch) {
				return false
			}
			continue
		}
		if !isLetter(ch) && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func digitsOnly(s string) bool {
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func compareNumeric(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// publishedBefore reports whether candidate should replace current as the earlier version.
// A real timestamp wins over an empty one. Equal timestamps fall back to the version string.
func publishedBefore(candidate, current catalogVersion) bool {
	return preferPublished(candidate, current, true)
}

// publishedLater reports whether candidate should replace current as the later version.
// A real timestamp wins over an empty one. Equal timestamps fall back to the version string.
func publishedLater(candidate, current catalogVersion) bool {
	return preferPublished(candidate, current, false)
}

func preferPublished(candidate, current catalogVersion, earlier bool) bool {
	candidateEmpty := candidate.publishedAt == ""
	currentEmpty := current.publishedAt == ""
	if candidateEmpty != currentEmpty {
		return currentEmpty
	}
	if !candidateEmpty && candidate.publishedAt != current.publishedAt {
		if earlier {
			return candidate.publishedAt < current.publishedAt
		}
		return candidate.publishedAt > current.publishedAt
	}
	return candidate.version < current.version
}

// earlierPublishedAt keeps the earlier real timestamp. An empty value loses to a real one.
func earlierPublishedAt(current, candidate string) string {
	if candidate == "" {
		return current
	}
	if current == "" || candidate < current {
		return candidate
	}
	return current
}
