package api

import (
	"encoding/json"
)

// TaskInfoResponse holds data returned by a tasks API response
type AdminTaskInfoResponse struct {
	UUID       string          `json:"uuid"`        // UUID of the object
	Status     string          `json:"status"`      // Status of task (running, failed, completed, canceled, pending)
	Typename   string          `json:"typename"`    // Type of task (e.g. introspect, completed)
	QueuedAt   string          `json:"queued_at"`   // Timestamp task was queued at
	StartedAt  string          `json:"started_at"`  // Timestamp task started running at
	FinishedAt string          `json:"finished_at"` // Timestamp task finished running at
	Error      string          `json:"error"`       // Error thrown while running task
	OrgId      string          `json:"org_id"`      // Organization ID of the owner
	AccountId  string          `json:"account_id"`  // Account ID of the owner
	Payload    json.RawMessage `json:"payload"`     // Payload of task
}

type AdminTaskInfoCollectionResponse struct {
	Data  []AdminTaskInfoResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata        `json:"meta"`  // Metadata about the request
	Links Links                   `json:"links"` // Links to other pages of results
}

func (t *AdminTaskInfoCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	t.Meta = meta
	t.Links = links
}
