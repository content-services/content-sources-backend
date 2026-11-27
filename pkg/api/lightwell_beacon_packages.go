package api

import "time"

type LightwellBeaconPackage struct {
	Ecosystem               string           `json:"ecosystem"`
	ComponentName           string           `json:"component_name"`
	ComponentVersion        string           `json:"component_version"`
	VulnerabilityCount      int64            `json:"vulnerability_count"`
	HighestSeverity         *string          `json:"highest_severity" extensions:"x-nullable"`
	SeverityCounts          map[string]int64 `json:"severity_counts"`
	StatusCounts            map[string]int64 `json:"status_counts"`
	HighestPublishedVersion *string          `json:"highest_published_version" extensions:"x-nullable"`
}

type LightwellBeaconVulnerabilitySummary struct {
	Count         int64            `json:"count"`
	CriticalCount int64            `json:"critical_count"`
	EmbargoCount  int64            `json:"embargo_count"`
	StatusCounts  map[string]int64 `json:"status_counts"`
}

type LightwellBeaconPackageMeta struct {
	ResponseMetadata
	VulnerabilitySummary LightwellBeaconVulnerabilitySummary `json:"vulnerability_summary"`
}

type LightwellBeaconPackageCollection struct {
	Data  []LightwellBeaconPackage   `json:"data"`
	Meta  LightwellBeaconPackageMeta `json:"meta"`
	Links Links                      `json:"links"`
}

func (r *LightwellBeaconPackageCollection) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta.ResponseMetadata = meta
	r.Links = links
}

type LightwellBeaconPackageVulnerability struct {
	UUID              string    `json:"uuid"`
	VulnerabilityID   string    `json:"vulnerability_id"`
	Title             *string   `json:"title,omitempty"`
	Severity          string    `json:"severity"`
	Cvss              *float64  `json:"cvss,omitempty"`
	Status            string    `json:"status"`
	LastUpdated       time.Time `json:"last_updated"`
	ResolutionReason  *string   `json:"resolution_reason,omitempty"`
	PublishedVersions []string  `json:"published_versions"`
}

type LightwellBeaconPackageVulnerabilityMeta struct {
	ResponseMetadata
	StatusCounts            map[string]int64 `json:"status_counts"`
	VulnerabilityCount      int64            `json:"vulnerability_count"`
	HighestSeverity         *string          `json:"highest_severity" extensions:"x-nullable"`
	HighestPublishedVersion *string          `json:"highest_published_version" extensions:"x-nullable"`
}

type LightwellBeaconPackageVulnerabilityCollection struct {
	Data  []LightwellBeaconPackageVulnerability   `json:"data"`
	Meta  LightwellBeaconPackageVulnerabilityMeta `json:"meta"`
	Links Links                                   `json:"links"`
}

func (r *LightwellBeaconPackageVulnerabilityCollection) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta.ResponseMetadata = meta
	r.Links = links
}
