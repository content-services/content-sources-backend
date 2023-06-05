package api

// TaskInfoResponse holds data returned by a tasks API response
type TaskInfoResponse struct {
	UUID      string `json:"uuid"`       // UUID of the object
	Status    string `json:"status"`     // Status of task (running, failed, completed, canceled, pending)
	CreatedAt string `json:"created_at"` // Timestamp of task creation
	EndedAt   string `json:"ended_at"`   // Timestamp task ended running at
	Error     string `json:"error"`      // Error thrown while running task
	OrgId     string `json:"org_id"`     // Organization ID of the owner
}

type TaskInfoCollectionResponse struct {
	Data  []TaskInfoResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata   `json:"meta"`  // Metadata about the request
	Links Links              `json:"links"` // Links to other pages of results
}

func (t *TaskInfoCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	t.Meta = meta
	t.Links = links
}
