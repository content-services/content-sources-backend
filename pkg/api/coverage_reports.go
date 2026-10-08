package api

import "time"

// CoverageReportResponse represents the coverage report
type CoverageReportResponse struct {
	SkippedEntries           *int                       `json:"skipped_entries,omitempty"` // Package entries skipped because no usable identity could be extracted; absent for older reports
	UUID                     string                     `json:"uuid"`
	Status                   string                     `json:"status"`                        // Coverage analysis task status
	InputFormat              string                     `json:"input_format"`                  // Detected manifest format
	CreatedAt                time.Time                  `json:"created_at"`                    // Timestamp when the report was created
	CompletedAt              *time.Time                 `json:"completed_at"`                  // Timestamp when coverage analysis finished
	Total                    int                        `json:"total"`                         // Total packages parsed from the manifest
	ExactMatches             int                        `json:"exact_matches"`                 // Number of packages with name and version found
	PartialMatches           int                        `json:"partial_matches"`               // Number of packages with name found but not version
	Unmatched                int                        `json:"unmatched"`                     // Number of packages with name not found
	EcosystemCoverageSummary []EcosystemCoverageSummary `json:"ecosystem_coverage_summary"`    // Per-ecosystem breakdown
	CveSummary               CveCount                   `json:"cve_summary"`                   // Count of CVEs fixed in remediated repos across covered packages, by severity
	AnalysisTaskError        string                     `json:"analysis_task_error,omitempty"` // Error if coverage analysis task failed
	AnalysisTaskUUID         string                     `json:"analysis_task_uuid"`            // UUID of the coverage analysis task
}

// CveCount represents a count of CVEs bucketed by severity
type CveCount struct {
	Critical  int `json:"critical"`
	Important int `json:"important"`
	Moderate  int `json:"moderate"`
	Low       int `json:"low"`
}

// CveRange represents the min and max CVSS severity scores across a package's CVEs
type CveRange struct {
	Low  float32 `json:"low"`  // Lowest CVSS severity score among the package's CVEs
	High float32 `json:"high"` // Highest CVSS severity score among the package's CVEs
}

// EcosystemCoverageSummary represents the ecosystem breakdown in a coverage report
type EcosystemCoverageSummary struct {
	Ecosystem      string `json:"ecosystem"`
	Supported      bool   `json:"supported"` // Whether the ecosystem is present in the Lightwell catalog.
	Total          int    `json:"total"`
	ExactMatches   int    `json:"exact_matches"`
	PartialMatches int    `json:"partial_matches"`
	Unmatched      int    `json:"unmatched"`
}

// CoverageReportPackageResponse represents a package in a coverage report
type CoverageReportPackageResponse struct {
	Name        string `json:"name"`         // Package name from the manifest
	Version     string `json:"version"`      // Package version from the manifest
	Ecosystem   string `json:"ecosystem"`    // Ecosystem of the package
	Covered     bool   `json:"covered"`      // Whether the package is covered (true = exact or partial match)
	MatchStatus string `json:"match_status"` // Match status of the package (exact, partial, none)

	CveCount CveCount  `json:"cve_count"`           // Count of CVEs fixed in remediated repos for this package, by severity
	CveRange *CveRange `json:"cve_range,omitempty"` // Min/max CVSS severity score across this package's CVEs; omitted when the package has no CVEs
}

// CoverageReportPackageCollectionResponse represents the paginated response for packages in a coverage report
type CoverageReportPackageCollectionResponse struct {
	Data  []CoverageReportPackageResponse `json:"data"`  // List of packages
	Meta  ResponseMetadata                `json:"meta"`  // Pagination metadata
	Links Links                           `json:"links"` // Navigation links
}

func (r *CoverageReportPackageCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

// ListCoverageReportPackagesRequest represents the request for listing packages in a coverage report
type ListCoverageReportPackagesRequest struct {
	Ecosystem   string `query:"ecosystem"`    // Optional filter for ecosystem
	Search      string `query:"search"`       // Optional filter for package name
	MatchStatus string `query:"match_status"` // Optional filter for match status (exact, partial, none)
}

// CreateCoverageReportRequest represents the request for creating a coverage report
type CreateCoverageReportRequest struct {
	File string `form:"file" validate:"required"` // Manifest file
}
