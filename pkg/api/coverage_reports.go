package api

import "time"

// CoverageReportResponse represents the coverage report
type CoverageReportResponse struct {
	UUID                     string                     `json:"uuid"`
	Status                   string                     `json:"status"`                               // Coverage analysis task status
	InputFormat              string                     `json:"input_format,omitempty"`               // Detected manifest format
	CreatedAt                time.Time                  `json:"created_at"`                           // Timestamp when the report was created
	CompletedAt              *time.Time                 `json:"completed_at,omitempty"`               // Timestamp when coverage analysis finished
	Total                    int                        `json:"total,omitempty"`                      // Total packages parsed from the manifest
	ExactMatches             int                        `json:"exact_matches,omitempty"`              // Number of packages with name and version found
	PartialMatches           int                        `json:"partial_matches,omitempty"`            // Number of packages with name found but not version
	Unmatched                int                        `json:"unmatched,omitempty"`                  // Number of packages with name not found
	EcosystemCoverageSummary []EcosystemCoverageSummary `json:"ecosystem_coverage_summary,omitempty"` // Per-ecosystem breakdown
	AnalysisTaskError        string                     `json:"analysis_task_error,omitempty"`        // Error if coverage analysis task failed
	AnalysisTaskUUID         string                     `json:"analysis_task_uuid,omitempty"`         // UUID of the coverage analysis task
}

// EcosystemCoverageSummary represents the ecosystem breakdown in a coverage report
type EcosystemCoverageSummary struct {
	Ecosystem      string `json:"ecosystem"`
	Total          int    `json:"total"`
	ExactMatches   int    `json:"exact_matches"`
	PartialMatches int    `json:"partial_matches"`
	Unmatched      int    `json:"unmatched"`
}

// CoverageReportPackageItem represents a package in a coverage report
type CoverageReportPackageItem struct {
	Name      string `json:"name"`      // Package name from the manifest
	Ecosystem string `json:"ecosystem"` // Ecosystem of the package
	Status    string `json:"status"`    // Package match status (in_network, not_in_network)
}

// CoverageReportPackagesResponse represents the paginated response for packages in a coverage report
type CoverageReportPackagesResponse struct {
	Results []CoverageReportPackageItem `json:"results"`
	Total   int                         `json:"total"`
	Limit   int                         `json:"limit"`
	Offset  int                         `json:"offset"`
}

// ListCoverageReportPackagesRequest represents the request for listing packages in a coverage report
type ListCoverageReportPackagesRequest struct {
	UUID      string `param:"uuid" validate:"required"` // Identifier of the coverage report
	Search    string `query:"search"`                   // Optional filter for package name
	Ecosystem string `query:"ecosystem"`                // Optional filter for ecosystem
	Status    string `query:"status"`                   // Optional filter for package match status
}

// CreateCoverageReportRequest represents the request for creating a coverage report
type CreateCoverageReportRequest struct {
	File string `form:"file" validate:"required"` // Manifest file
}
