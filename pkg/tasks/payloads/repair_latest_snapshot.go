package payloads

const Repair = "Repair"

type RepairPayload struct {
	RepositoryConfigUUID string
	PulpRepoVersionHref  *string
	RepairTaskHref       *string
}
