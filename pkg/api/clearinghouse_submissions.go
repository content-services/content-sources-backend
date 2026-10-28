package api

import "time"

const (
	ClearinghouseSubmissionStatusPending   = "pending"
	ClearinghouseSubmissionStatusSubmitted = "submitted"
	ClearinghouseSubmissionStatusProcessed = "processed"
)

// ClearinghouseSubmissionFile is a file stored with a clearinghouse submission.
type ClearinghouseSubmissionFile struct {
	UUID      string    `json:"uuid"`
	Filename  string    `json:"filename"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// ClearinghouseSubmission is one customer clearinghouse upload and the files stored with it.
type ClearinghouseSubmission struct {
	UUID               string                        `json:"uuid"`
	OrgID              string                        `json:"org_id"`
	Username           string                        `json:"username"`
	Name               string                        `json:"name"`
	CreatedAt          time.Time                     `json:"created_at"`
	SubmittedAt        *time.Time                    `json:"submitted_at,omitempty"`
	SubmittedBy        *string                       `json:"submitted_by,omitempty"`
	ProcessedAt        *time.Time                    `json:"processed_at,omitempty"`
	ProcessedBy        *string                       `json:"processed_by,omitempty"`
	VulnerabilityCount *int                          `json:"vulnerability_count,omitempty"`
	Status             string                        `json:"status"`
	ErrorMessage       *string                       `json:"error_message,omitempty"`
	Files              []ClearinghouseSubmissionFile `json:"files"`
}

// ReportedVulnerability is a single reported vulnerability within an intake submission.
type ReportedVulnerability struct {
	VulnerabilityID    string  `json:"vulnerability_id"`
	Packageurl         string  `json:"packageurl"`
	ComponentName      string  `json:"component_name"`
	ComponentVersion   string  `json:"component_version"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
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
	Data  []ClearinghouseSubmission `json:"data"`
	Meta  ResponseMetadata          `json:"meta"`
	Links Links                     `json:"links"`
}

func (r *ClearinghouseSubmissionCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
