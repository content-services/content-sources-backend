package payloads

type UpdateTemplateContentPayload struct {
	TemplateUUID              string
	RepoConfigUUIDs           []string
	TriggeredByRepositoryUUID *string
	PoolID                    *string // Add during task runtime
}
