package api

// TaskInfoResponse holds data returned by a tasks API response
type TaskInfoResponse struct {
	UUID           string   `json:"uuid"`            // UUID of the object
	Status         string   `json:"status"`          // Status of task (running, failed, completed, canceled, pending)
	CreatedAt      string   `json:"created_at"`      // Timestamp of task creation
	EndedAt        string   `json:"ended_at"`        // Timestamp task ended running at
	Error          string   `json:"error"`           // Error thrown while running task
	OrgId          string   `json:"org_id"`          // Organization ID of the owner
	Typename       string   `json:"type"`            // Type of task
	RepoConfigName string   `json:"repository_name"` // Name of the associated repository
	RepoConfigUUID string   `json:"repository_uuid"` // UUID of the associated repository
	Dependencies   []string `json:"dependencies"`    // UUIDs of parent tasks
	Dependents     []string `json:"dependents"`      // UUIDs of child tasks
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

type TaskInfoFilterData struct {
	Status           string `query:"status" json:"status"`
	Typename         string `query:"type" json:"type"`
	RepoConfigUUID   string `query:"repository_uuid" json:"repository_uuid"`
	ExcludeRedHatOrg bool   `json:"exclude_red_hat_org"`
}
