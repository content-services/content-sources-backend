package api

import (
	"regexp"
	"time"
)

var (
	cveAdvisoryNameRE = regexp.MustCompile(`CVE-\d{4}-\d+`)
	lwAdvisoryNameRE  = regexp.MustCompile(`LW-\d{4}-\d+`)
)

type LightwellAdvisoryResponse struct {
	AdvisoryID     string     `json:"advisory_id"`
	AdvisoryName   string     `json:"advisory_name"`
	Severity       string     `json:"severity"`
	SeverityScore  float32    `json:"severity_score"`
	Summary        string     `json:"summary"`
	Details        string     `json:"details"`
	ReferenceURLs  []string   `json:"reference_urls"`
	PackageName    string     `json:"package_name"`
	PackageVersion string     `json:"package_version"`
	FixedVersions  []string   `json:"fixed_versions"`
	Repository     string     `json:"repository"`
	Published      *time.Time `json:"published"`
	Modified       *time.Time `json:"modified"`
	Aliases        []string   `json:"aliases"`
	SchemaVersion  string     `json:"schema_version"`
	Source         string     `json:"source"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type LightwellAdvisoryCollectionResponse struct {
	Data  []LightwellAdvisoryResponse `json:"data"`
	Meta  ResponseMetadata            `json:"meta"`
	Links Links                       `json:"links"`
}

func (r *LightwellAdvisoryCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

type LightwellAdvisoryFilterData struct {
	Repository     string `query:"repository"`
	PackageName    string `query:"package_name"`
	PackageVersion string `query:"package_version"`
	SeverityMin    string `query:"severity_min"`
	CveID          string `query:"cve_id"`
	Name           string `query:"name"`
	LatestRelease  bool   `query:"latest_release"`
}

// LightwellAdvisoryName returns the trimmed CVE or LW identifier from an OSV advisory id.
// If neither pattern is present, the full advisory id is returned.
func LightwellAdvisoryName(advisoryID string) string {
	if m := cveAdvisoryNameRE.FindString(advisoryID); m != "" {
		return m
	}
	if m := lwAdvisoryNameRE.FindString(advisoryID); m != "" {
		return m
	}
	return advisoryID
}
