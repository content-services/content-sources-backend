package utils

import (
	"slices"
	"strings"
)

// SplitFeatureNames splits a comma-separated repository feature_name value into
// non-empty trimmed tokens (order preserved).
func SplitFeatureNames(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// NormalizeUniqueSortedFeatureNamesFromCSV parses comma-separated feature names,
// deduplicates, and sorts for stable guard and comparison keys.
func NormalizeUniqueSortedFeatureNamesFromCSV(csv string) []string {
	return NormalizeUniqueSorted(SplitFeatureNames(csv))
}

// NormalizeUniqueSorted deduplicates names, trims, and sorts ascending.
func NormalizeUniqueSorted(names []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// EntitledToAnyFeatureInCSV returns true if entitled contains at least one token
// from featureNameCSV, or if featureNameCSV has no tokens (no feature gate).
func EntitledToAnyFeatureInCSV(entitled []string, featureNameCSV string) bool {
	for _, name := range SplitFeatureNames(featureNameCSV) {
		if slices.Contains(entitled, name) {
			return true
		}
	}
	return len(SplitFeatureNames(featureNameCSV)) == 0
}

// ImportFeatureMatches returns true if any token in repoFeatureCSV equals a value
// in filterFeatures, or if repoFeatureCSV has no tokens.
func ImportFeatureMatches(repoFeatureCSV string, filterFeatures []string) bool {
	repoNames := NormalizeUniqueSorted(SplitFeatureNames(repoFeatureCSV))
	if len(repoNames) == 0 {
		return true
	}
	for _, f := range filterFeatures {
		if slices.Contains(repoNames, f) {
			return true
		}
	}
	return false
}

// AllStringsIn returns true if every name in names is contained in list (vacuously true if names is empty).
func AllStringsIn(names, list []string) bool {
	for _, n := range names {
		if !slices.Contains(list, n) {
			return false
		}
	}
	return true
}
