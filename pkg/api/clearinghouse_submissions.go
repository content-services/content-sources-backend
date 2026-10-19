package api

import "time"

const (
	ClearinghouseSubmissionStatusUploading = "uploading"
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
	Filename           string                        `json:"filename"`
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
