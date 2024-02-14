package payloads

const UpdateTemplateDistributions = "update-template-distributions"

type UpdateTemplateDistributionsPayload struct {
	TemplateUUID    string
	TemplateDate    string
	RepoConfigUUIDs []string
}
