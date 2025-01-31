package payloads

const Snapshot = "snapshot"

type SnapshotPayload struct {
	SnapshotIdent        *string
	SnapshotUUID         *string
	SyncTaskHref         *string
	PublicationTaskHref  *string
	DistributionTaskHref *string
}
