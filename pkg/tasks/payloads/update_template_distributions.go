package payloads

const UpdateTemplateDistributions = "update-template-distributions"

type UpdateTemplateDistributionsPayload struct {
	TemplateUUID    string
	RepoConfigUUIDs []string
}
