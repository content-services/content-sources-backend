package sync

import (
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/utils"
)

type PublishedAdvisory struct {
	RepoName      string
	AdvisoryID    string
	PackageName   string
	FixedVersions []string
}

// publishedOnNetwork reports whether an advisory publishes this vulnerability for its package, language, and version.
func publishedOnNetwork(v Vulnerability, advisories []PublishedAdvisory) bool {
	componentName := strings.TrimSpace(v.ComponentName)
	componentVersion := strings.TrimSpace(v.ComponentVersion)
	if componentName == "" || componentVersion == "" || strings.TrimSpace(v.VulnerabilityID) == "" || v.Language == nil {
		return false
	}
	for _, advisory := range advisories {
		if !packageMatches(advisory.PackageName, v) {
			continue
		}
		if !repoMatchesLanguage(advisory.RepoName, *v.Language) {
			continue
		}
		if versionPresent(advisory, v.VulnerabilityID, componentVersion) {
			return true
		}
	}
	return false
}

func packageMatches(advisoryPackage string, vulnerability Vulnerability) bool {
	advisoryPackage = strings.TrimSpace(advisoryPackage)
	if advisoryPackage == "" {
		return false
	}
	if strings.EqualFold(advisoryPackage, strings.TrimSpace(vulnerability.ComponentName)) {
		return true
	}
	if vulnerability.PURL != nil {
		parsed := utils.ParsePURL(strings.TrimSpace(*vulnerability.PURL))
		if parsed != nil && parsed.FullName() != "" {
			packageName := parsed.FullName()
			return strings.EqualFold(advisoryPackage, packageName)
		}
	}
	return false
}

// repoMatchesLanguage reports whether language appears as a path segment in repoName.
func repoMatchesLanguage(repoName, language string) bool {
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "" {
		return false
	}
	return slices.Contains(strings.Split(strings.ToLower(repoName), "/"), lang)
}

// versionPresent reports whether the advisory names this vulnerability and covers this version.
func versionPresent(advisory PublishedAdvisory, vulnID, version string) bool {
	after, ok := vulnIDInAdvisory(advisory.AdvisoryID, vulnID)
	if !ok {
		return false
	}
	if strings.HasPrefix(after, "-") && versionAt(after[1:], version) {
		return true
	}
	for _, fixed := range advisory.FixedVersions {
		if versionAt(fixed, version) {
			return true
		}
	}
	return false
}

// vulnIDInAdvisory reports whether vulnID appears as a whole token in advisoryID.
// A shorter ID such as LW-0000-0001 does not match LW-0000-00010.
func vulnIDInAdvisory(advisoryID, vulnID string) (after string, ok bool) {
	id := strings.ToUpper(strings.TrimSpace(vulnID))
	if id == "" {
		return "", false
	}
	haystack := strings.ToUpper(advisoryID)
	for start := 0; start <= len(haystack)-len(id); {
		idx := strings.Index(haystack[start:], id)
		if idx < 0 {
			return "", false
		}
		idx += start
		if !continuesID(haystack, idx-1) && !continuesID(haystack, idx+len(id)) {
			return advisoryID[idx+len(id):], true
		}
		start = idx + 1
	}
	return "", false
}

func continuesID(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// versionAt reports whether value is version or version followed by a non-numeric suffix.
func versionAt(value, version string) bool {
	if value == version {
		return true
	}
	if !strings.HasPrefix(value, version) || len(value) <= len(version) {
		return false
	}
	sep := value[len(version)]
	if sep != '.' && sep != '-' && sep != '+' {
		return false
	}
	return len(value) == len(version)+1 || value[len(version)+1] < '0' || value[len(version)+1] > '9'
}
