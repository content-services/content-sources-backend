package payloads

const Snapshot = "snapshot"

type SnapshotPayload struct {
	SnapshotIdent        *string
	SyncTaskHref         *string
	PublicationTaskHref  *string
	DistributionTaskHref *string
}
