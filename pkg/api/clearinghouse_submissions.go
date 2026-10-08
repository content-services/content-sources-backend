package api

import "time"

const (
	ClearinghouseSubmissionStatusValidating = "validating"
	ClearinghouseSubmissionStatusSubmitted  = "submitted"
	ClearinghouseSubmissionStatusFailed     = "failed"
)

// ClearinghouseSubmissionFile is a file stored with a clearinghouse submission.
type ClearinghouseSubmissionFile struct {
	UUID     string `json:"uuid"`
	Filename string `json:"filename"`
}

// ClearinghouseSubmission is one customer clearinghouse upload and the files stored with it.
type ClearinghouseSubmission struct {
	UUID               string                        `json:"uuid"`
	OrgID              string                        `json:"org_id"`
	AccountID          *string                       `json:"account_id,omitempty"`
	CreatedAt          time.Time                     `json:"created_at"`
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
