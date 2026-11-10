package api

import "time"

const (
	ClearinghouseSubmissionStatusPending   = "pending"
	ClearinghouseSubmissionStatusSubmitted = "submitted"
	ClearinghouseSubmissionStatusProcessed = "processed"
)

// ClearinghouseEcosystemUnknown is the ecosystem reported for packages whose packageurl does not parse as a Package URL.
const ClearinghouseEcosystemUnknown = "Unknown"

// Reasons a submission package is invalid, reported as invalid_reason.
const (
	// ClearinghouseInvalidReasonNotAPurl means the packageurl does not parse as a Package URL.
	ClearinghouseInvalidReasonNotAPurl = "not_a_purl"
	// ClearinghouseInvalidReasonMissingVersion means the packageurl is a Package URL without a version.
	ClearinghouseInvalidReasonMissingVersion = "missing_version"
)

// ClearinghouseSubmissionFile is a file stored with a clearinghouse submission.
type ClearinghouseSubmissionFile struct {
	UUID     string `json:"uuid"`
	Filename string `json:"filename"`
	Username string `json:"username"`
	// Submission is true for the file holding the intake submission JSON itself (not a reproducer or other attachment).
	Submission bool      `json:"submission"`
	CreatedAt  time.Time `json:"created_at"`
	// CanDelete is true when the caller can delete this file.
	CanDelete bool `json:"can_delete"`
	// CanDownload is true when the caller can download this file.
	CanDownload bool `json:"can_download"`
}

// ClearinghouseSubmission is one customer clearinghouse upload and the files stored with it.
type ClearinghouseSubmission struct {
	UUID               string     `json:"uuid"`
	OrgID              string     `json:"org_id"`
	Username           string     `json:"username"`
	Name               string     `json:"name"`
	CreatedAt          time.Time  `json:"created_at"`
	SubmittedAt        *time.Time `json:"submitted_at,omitempty"`
	SubmittedBy        *string    `json:"submitted_by,omitempty"`
	ProcessedAt        *time.Time `json:"processed_at,omitempty"`
	ProcessedBy        *string    `json:"processed_by,omitempty"`
	VulnerabilityCount *int       `json:"vulnerability_count,omitempty"`
	Status             string     `json:"status"`
	ErrorMessage       *string    `json:"error_message,omitempty"`
	// PackageSummary is the per-ecosystem breakdown of the packages in the submission JSON. Absent for submissions created before package parsing was added.
	PackageSummary *ClearinghousePackageSummary `json:"package_summary,omitempty"`
	// CanDelete is true when the caller can delete the submission.
	CanDelete bool `json:"can_delete"`
	// CanDownload is true when the caller can download the submission and its files.
	CanDownload bool `json:"can_download"`
	// CanAddReproducers is true when the caller can add files to the submission.
	CanAddReproducers bool `json:"can_add_reproducers"`
	// CanSubmit is true when the caller can submit the submission.
	CanSubmit bool `json:"can_submit"`
	// CanProcess is true when the caller can mark the submission as processed.
	CanProcess bool                          `json:"can_process"`
	Files      []ClearinghouseSubmissionFile `json:"files"`
}

// ClearinghouseSubmissionWithCID is a clearinghouse submission returned by list and get.
type ClearinghouseSubmissionWithCID struct {
	ClearinghouseSubmission
	// Customer identifier. Only visible to internal users.
	CID string `json:"cid"`
}

// ReportedVulnerability is a single reported vulnerability within an intake submission.
type ReportedVulnerability struct {
	VulnerabilityID    string  `json:"vulnerability_id" validate:"required"`
	Packageurl         string  `json:"packageurl" validate:"required"`
	ComponentName      string  `json:"component_name"`
	ComponentVersion   string  `json:"component_version"`
	Title              string  `json:"title" validate:"required"`
	Description        string  `json:"description" validate:"required"`
	CvssSeverity       string  `json:"cvss_severity"`
	CvssScore          float64 `json:"cvss_score"`
	Cwe                *string `json:"cwe,omitempty"`
	CvssVector         *string `json:"cvss_vector,omitempty"`
	ExploitTested      *bool   `json:"exploit_tested,omitempty"`
	ReproducerFilename *string `json:"reproducer_filename,omitempty"`
	CustomerPriority   *string `json:"customer_priority,omitempty"`
}

// IntakeSubmission is a vulnerability intake submission body posted to create a clearinghouse submission.
type IntakeSubmission struct {
	// Name is a user-facing label for the submission. Required.
	Name            string                  `json:"name"`
	Vulnerabilities []ReportedVulnerability `json:"vulnerabilities"`
}

// ClearinghouseSubmissionCollectionResponse is a paginated list of clearinghouse submissions.
type ClearinghouseSubmissionCollectionResponse struct {
	Data  []ClearinghouseSubmissionWithCID `json:"data"`
	Meta  ResponseMetadata                 `json:"meta"`
	Links Links                            `json:"links"`
}

func (r *ClearinghouseSubmissionCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

// ClearinghousePackageSummary summarizes the unique packages referenced by a submission's vulnerabilities.
type ClearinghousePackageSummary struct {
	// Total is the number of unique packages. A package is unique by ecosystem, namespace, name and version; invalid packageurls are unique by their value.
	Total int `json:"total"`
	// InvalidPurls is the number of unique packageurls that are invalid: they do not parse as a Package URL (counted under the Unknown ecosystem) or have no version (counted under their ecosystem).
	InvalidPurls int `json:"invalid_purls"`
	// Ecosystems is the per-ecosystem breakdown, largest first.
	Ecosystems []ClearinghousePackageEcosystemSummary `json:"ecosystems"`
}

// ClearinghousePackageEcosystemSummary is the number of unique packages in one ecosystem.
type ClearinghousePackageEcosystemSummary struct {
	Ecosystem string `json:"ecosystem"`
	Total     int    `json:"total"`
}

// ClearinghouseSubmissionPackage is one unique package referenced by a submission's vulnerabilities.
type ClearinghouseSubmissionPackage struct {
	// Packageurl is the packageurl as submitted.
	Packageurl string `json:"packageurl"`
	// Ecosystem is the package ecosystem, or Unknown when the packageurl does not parse as a Package URL.
	Ecosystem string `json:"ecosystem"`
	// Name is the package name including its namespace. Empty when the packageurl does not parse as a Package URL.
	Name string `json:"name"`
	// Version is the package version. Empty when the packageurl does not parse as a Package URL or has no version.
	Version string `json:"version"`
	// ValidPurl is false when the packageurl does not parse as a Package URL or has no version.
	ValidPurl bool `json:"valid_purl"`
	// InvalidReason is a fixed code saying why the package is invalid. Set only when valid_purl is false.
	InvalidReason *string `json:"invalid_reason,omitempty" enums:"not_a_purl,missing_version"`
	// InvalidDetail is a human-readable explanation of invalid_reason, such as the Package URL parser error. Set only when valid_purl is false.
	InvalidDetail *string `json:"invalid_detail,omitempty"`
}

// ClearinghouseSubmissionPackageCollectionResponse is a paginated list of the packages in a submission.
type ClearinghouseSubmissionPackageCollectionResponse struct {
	Data  []ClearinghouseSubmissionPackage `json:"data"`
	Meta  ResponseMetadata                 `json:"meta"`
	Links Links                            `json:"links"`
}

func (r *ClearinghouseSubmissionPackageCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
