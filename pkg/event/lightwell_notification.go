package event

import (
	"sort"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
)

const (
	LightwellNotificationBundle      = "lightwell"
	LightwellNotificationApplication = "lightwell"
	LightwellNotificationVersion     = "2.0.0"
)

const (
	LightwellEventTypeJavaRemediated    = "java-remediated"
	LightwellEventTypeJavaPredisclosure = "java-predisclosure"
)

const (
	SeverityCritical  = "critical"
	SeverityImportant = "important"
	SeverityModerate  = "moderate"
	SeverityLow       = "low"
)

func LightwellPackageLink(repoName, pkgName string) string {
	slug := strings.TrimPrefix(repoName, "lightwell/")
	slug = strings.ReplaceAll(slug, "/", "-")

	pkgPath := pkgName
	if parts := strings.SplitN(pkgName, ":", 2); len(parts) == 2 {
		pkgPath = parts[0] + "/" + parts[1]
	}

	return strings.TrimSuffix(config.Get().Options.ExternalURL, "/") + "/lightwell/" + slug + "/" + pkgPath
}

func LightwellCVEURL(advisoryID string) string {
	return strings.TrimSuffix(config.Get().Options.ExternalURL, "/") + "/api/lightwell/cves/" + advisoryID + ".json"
}

// LightwellEventType maps a repository name to its notification event type.
func LightwellEventType(repoName string) string {
	switch repoName {
	case "lightwell/java/remediated":
		return LightwellEventTypeJavaRemediated
	case "lightwell/java/predisclosure":
		return LightwellEventTypeJavaPredisclosure
	default:
		return ""
	}
}

type LightwellNotificationInput struct {
	PackageName   string
	AdvisoryID    string
	Severity      string
	FixedVersions []string
	ReferenceURLs []string
}

type LightwellPackagePayload struct {
	PackageLink string                    `json:"package_link"`
	PackageName string                    `json:"package_name"`
	Releases    []LightwellReleasePayload `json:"releases"`
}

type LightwellReleasePayload struct {
	RelatedCVE   []LightwellCVEPayload  `json:"related_cve"`
	ReleaseNames []LightwellReleaseName `json:"release_names"`
}

type LightwellCVEPayload struct {
	CVE      string `json:"cve"`
	Severity string `json:"severity"`
	URL      string `json:"url"`
}

type LightwellReleaseName struct {
	Name string `json:"name"`
}

// BuildLightwellNotificationEvents transforms a flat list of advisory data into notification events, one event per unique package.
func BuildLightwellNotificationEvents(repoName string, advisories []LightwellNotificationInput) []NotificationEvent {
	grouped := groupByPackage(advisories)

	events := make([]NotificationEvent, 0, len(grouped))

	// For each package (event), we must build the payload
	// Each payload contains a package link, name, and a releases list
	for pkgName, inputs := range grouped {
		payload := LightwellPackagePayload{
			PackageLink: LightwellPackageLink(repoName, pkgName),
			PackageName: pkgName,
			Releases:    buildReleases(inputs),
		}
		events = append(events, NotificationEvent{
			Metadata: map[string]any{},
			Payload:  payload,
		})
	}
	return events
}

// groupByPackage groups inputs by package name.
// A package could have multiple advisories, creates a map: package_name -> []advisories
func groupByPackage(advisories []LightwellNotificationInput) map[string][]LightwellNotificationInput {
	result := make(map[string][]LightwellNotificationInput)
	for _, d := range advisories {
		result[d.PackageName] = append(result[d.PackageName], d)
	}
	return result
}

// buildReleases groups advisories by their fixed versions into release payloads.
func buildReleases(inputs []LightwellNotificationInput) []LightwellReleasePayload {
	grouped := groupByFixedVersions(inputs)

	releases := make([]LightwellReleasePayload, 0, len(grouped))
	for _, group := range grouped {
		releases = append(releases, LightwellReleasePayload{
			RelatedCVE:   buildCVEPayloads(group),
			ReleaseNames: buildReleaseNames(group[0].FixedVersions),
		})
	}
	return releases
}

// groupByFixedVersions groups inputs that share the same set of fixed versions.
func groupByFixedVersions(inputs []LightwellNotificationInput) map[string][]LightwellNotificationInput {
	result := make(map[string][]LightwellNotificationInput)
	for _, input := range inputs {
		key := buildVersionsKey(input.FixedVersions)
		result[key] = append(result[key], input)
	}
	return result
}

// buildCVEPayloads converts advisory inputs into CVE notification payloads.
func buildCVEPayloads(inputs []LightwellNotificationInput) []LightwellCVEPayload {
	cves := make([]LightwellCVEPayload, len(inputs))
	for i, input := range inputs {
		cves[i] = LightwellCVEPayload{
			CVE:      input.AdvisoryID,
			Severity: CVSSScoreToLabel(input.Severity),
			URL:      LightwellCVEURL(input.AdvisoryID),
		}
	}
	return cves
}

// buildReleaseNames converts version strings into release name payloads.
func buildReleaseNames(versions []string) []LightwellReleaseName {
	names := make([]LightwellReleaseName, len(versions))
	for i, v := range versions {
		names[i] = LightwellReleaseName{Name: v}
	}
	return names
}

// buildVersionsKey produces a stable key from a set of version strings for grouping.
func buildVersionsKey(versions []string) string {
	sorted := make([]string, len(versions))
	copy(sorted, versions)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

// CVSSScoreToLabel converts a numeric CVSS score string to a severity label.
func CVSSScoreToLabel(scoreStr string) string {
	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil || score <= 0 {
		return SeverityLow
	}
	switch {
	case score >= 9.0:
		return SeverityCritical
	case score >= 7.0:
		return SeverityImportant
	case score >= 4.0:
		return SeverityModerate
	default:
		return SeverityLow
	}
}

// MaximumSeverity returns the highest severity label across a set of advisory inputs.
func MaximumSeverity(advisories []LightwellNotificationInput) string {
	var maxScore float64
	for _, d := range advisories {
		score, err := strconv.ParseFloat(d.Severity, 64)
		if err == nil && score > maxScore {
			maxScore = score
		}
	}
	return CVSSScoreToLabel(strconv.FormatFloat(maxScore, 'f', 1, 64))
}
