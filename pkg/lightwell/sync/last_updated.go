package sync

import (
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/dao"
)

const (
	stageLightwellNetwork = "Lightwell Network"
	stageUnremediated     = "Unremediated"
)

// VulnerabilityState is the stage and last_updated already stored for an IVT.
type VulnerabilityState struct {
	Stage       string
	LastUpdated time.Time
}

type storedVulnerability struct {
	Found       bool
	Stage       string
	LastUpdated time.Time
}

// applyLastUpdated freezes last_updated once an IVT is shipped or closed
// without a fix.
//
// Lightwell Network uses the latest publish time of the package versions that
// qualified the IVT, so a later Jira comment or contributor edit does not move
// it. Unremediated keeps the Jira updated time from the sync that first
// observed the transition, and leaves it alone while the stage stays there.
func applyLastUpdated(v *Vulnerability, stored storedVulnerability, publishedAt time.Time, hasPublishedAt bool) {
	if v == nil {
		return
	}
	switch v.Stage {
	case stageUnremediated:
		if stored.Found && stored.Stage == stageUnremediated {
			v.LastUpdated = stored.LastUpdated
		}
	case stageLightwellNetwork:
		switch {
		case hasPublishedAt:
			v.LastUpdated = publishedAt.UTC()
		case stored.Found && stored.Stage == stageLightwellNetwork:
			v.LastUpdated = stored.LastUpdated
		}
	}
}

// latestPackagePublishedAt returns the latest publish time among rows for this
// component. Rows for a different component are ignored. Python names use the
// publication catalog's separator rules, so typing_extensions matches
// typing-extensions.
func latestPackagePublishedAt(component string, language *string, rows []dao.PackagePublishTime) (time.Time, bool) {
	var latest time.Time
	found := false
	for _, row := range rows {
		if !componentMatches(component, language, row.Group, row.Name) {
			continue
		}
		parsed, ok := parsePublishedAt(row.PublishedAt)
		if !ok {
			continue
		}
		if !found || parsed.After(latest) {
			latest = parsed
			found = true
		}
	}
	return latest, found
}

func componentMatches(component string, language *string, group, name string) bool {
	component = strings.TrimSpace(component)
	name = strings.TrimSpace(name)
	group = strings.TrimSpace(group)
	if component == "" || name == "" {
		return false
	}
	if pythonComponent(language) {
		if group != "" {
			return false
		}
		normalized := normalizePythonPackageName(component)
		return normalized != "" && normalized == normalizePythonPackageName(name)
	}
	if strings.EqualFold(component, name) {
		return true
	}
	if group == "" {
		return false
	}
	return strings.EqualFold(component, group+":"+name) || strings.EqualFold(component, group+"/"+name)
}

func pythonComponent(language *string) bool {
	if language == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(*language)) {
	case "python", "pypi":
		return true
	default:
		return false
	}
}

func parsePublishedAt(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}
