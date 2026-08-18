package api

import "time"

// CoverageReportResponse represents the coverage report
type CoverageReportResponse struct {
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
	AnalysisTaskError        string                     `json:"analysis_task_error,omitempty"` // Error if coverage analysis task failed
	AnalysisTaskUUID         string                     `json:"analysis_task_uuid"`            // UUID of the coverage analysis task
}

// EcosystemCoverageSummary represents the ecosystem breakdown in a coverage report
type EcosystemCoverageSummary struct {
	Ecosystem      string `json:"ecosystem"`
	Total          int    `json:"total"`
	ExactMatches   int    `json:"exact_matches"`
	PartialMatches int    `json:"partial_matches"`
	Unmatched      int    `json:"unmatched"`
}

// CoverageReportPackageResponse represents a package in a coverage report
type CoverageReportPackageResponse struct {
	Name      string `json:"name"`      // Package name from the manifest
	Version   string `json:"version"`   // Package version from the manifest
	Ecosystem string `json:"ecosystem"` // Ecosystem of the package
	Covered   bool   `json:"covered"`   // Whether the package is covered (true = exact or partial match)
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
	Covered   *bool  `query:"covered"`   // Optional filter for coverage status (true = covered, false = not covered)
	Ecosystem string `query:"ecosystem"` // Optional filter for ecosystem
	Search    string `query:"search"`    // Optional filter for package name
}

// CreateCoverageReportRequest represents the request for creating a coverage report
type CreateCoverageReportRequest struct {
	File string `form:"file" validate:"required"` // Manifest file
}
