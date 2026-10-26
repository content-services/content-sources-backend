package payloads

const CreatePartnerSnapshot = "create-partner-snapshot"

type CreatePartnerSnapshotPayload struct {
	PublicationHref   string
	RepositoryName    string
	RepositoryVersion string // OS version like "9"
	RepositoryArch    string // e.g., "x86_64", "aarch64"

	// Copr metadata for repository identification
	CoprProjectID      string
	CoprRepositoryHref string
	CoprBuildID        string

	// Resumability fields (for distribution creation)
	SnapshotIdent        *string
	SnapshotUUID         *string
	DistributionTaskHref *string
}
