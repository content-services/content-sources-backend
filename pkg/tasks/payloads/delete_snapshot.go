package payloads

type DeleteSnapshotsPayload struct {
	RepoUUID       string
	SnapshotsUUIDs []string
}
