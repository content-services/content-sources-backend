package utils

import (
	"slices"
	"strings"
)

// ParseFeatures parses feature names separated by comma or plus sign (both are
// treated as delimiters; plus is only for readability (not to be used inside a
// single feature name). Tokens are trimmed, empties dropped, deduplicated,
// and sorted ascending for stable guard and comparison keys.
func ParseFeatures(featureNames string) []string {
	if featureNames == "" {
		return nil
	}
	normalized := strings.ReplaceAll(featureNames, "+", ",")
	parts := strings.Split(normalized, ",")
	seen := make(map[string]struct{})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// AnyFeatureMatch returns true if featureNames has no parsed tokens (no feature gate),
// or if at least one parsed feature appears in list.
func AnyFeatureMatch(featureNames string, list []string) bool {
	names := ParseFeatures(featureNames)
	if len(names) == 0 {
		return true
	}
	for _, n := range names {
		if slices.Contains(list, n) {
			return true
		}
	}
	return false
}
