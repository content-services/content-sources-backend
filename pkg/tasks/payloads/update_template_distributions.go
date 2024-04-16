package payloads

const UpdateTemplateDistributions = "update-template-content"

type UpdateTemplateDistributionsPayload struct {
	TemplateUUID    string
	RepoConfigUUIDs []string
}
