package payloads

type UpdateTemplateContentPayload struct {
	TemplateUUID    string
	RepoConfigUUIDs []string
	PoolID          *string // Add during task runtime
}
