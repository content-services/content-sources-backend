package rhlw

import (
	"regexp"
	"strconv"
)

// Rank is a Lightwell rebuild identity. A missing novel or hotfix segment is 0.
// Compare left to right: Baseline, then Novel, then Hotfix.
type Rank struct {
	Baseline int
	Novel    int
	Hotfix   int
}

// Release is a version string that parsed as a Lightwell rebuild.
type Release struct {
	Version string
	Rank    Rank
}

// rhlw[.+-]<baseline> then optional n / hf segments.
// Matches maven (1.2.3.rhlw.00003.n00001.hf00001), python (1.2.3+rhlw.1.n1),
// and hyphen forms already stored (1.2.3.rhlw-00003, 1.2.3.rhlw-00000-n-00008).
// Does not match x_RHLW-CVE-… / x_RHLW-LW-… because those are not followed by digits.
var rebuildRE = regexp.MustCompile(`(?i)rhlw[.+-](\d+)(?:[.+-]n[.+-]?(\d+))?(?:[.+-]hf[.+-]?(\d+))?`)

// Parse extracts a rebuild rank from a version or advisory id.
// ok is false for upstream versions and for ids with no rebuild token.
func Parse(version string) (Rank, bool) {
	m := rebuildRE.FindStringSubmatch(version)
	if m == nil {
		return Rank{}, false
	}
	return Rank{
		Baseline: atoi(m[1]),
		Novel:    atoi(m[2]),
		Hotfix:   atoi(m[3]),
	}, true
}

// Releases collects unique rebuilds from fixed_versions, then from advisoryID
// if that id contains a rebuild token not already represented by rank.
func Releases(fixedVersions []string, advisoryID string) []Release {
	seenRank := make(map[Rank]struct{}, len(fixedVersions)+1)
	seenVersion := make(map[string]struct{}, len(fixedVersions)+1)
	out := make([]Release, 0, len(fixedVersions)+1)

	add := func(version string) {
		if version == "" {
			return
		}
		if _, dup := seenVersion[version]; dup {
			return
		}
		rank, ok := Parse(version)
		if !ok {
			return
		}
		if _, dup := seenRank[rank]; dup {
			return
		}
		seenVersion[version] = struct{}{}
		seenRank[rank] = struct{}{}
		out = append(out, Release{Version: version, Rank: rank})
	}

	for _, v := range fixedVersions {
		add(v)
	}
	add(advisoryID)
	return out
}

func atoi(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
