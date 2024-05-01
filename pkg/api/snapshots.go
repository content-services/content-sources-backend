package api

import (
	"time"
)

type SnapshotResponse struct {
	UUID           string           `json:"uuid"`
	CreatedAt      time.Time        `json:"created_at"`      // Datetime the snapshot was created
	RepositoryPath string           `json:"repository_path"` // Path to repository snapshot contents
	ContentCounts  map[string]int64 `json:"content_counts"`  // Count of each content type
	AddedCounts    map[string]int64 `json:"added_counts"`    // Count of each content type
	RemovedCounts  map[string]int64 `json:"removed_counts"`  // Count of each content type
	URL            string           `json:"url"`             // URL to the snapshot's content
}

type ListSnapshotByDateRequest struct {
	RepositoryUUIDS []string  `json:"repository_uuids"` // Repository UUIDs to find snapshots for
	Date            time.Time `json:"date"`             // Exact date to search by.
}

type ListSnapshotByDateResponse struct {
	Data []SnapshotForDate `json:"data"` // Requested Data
}

type SnapshotForDate struct {
	RepositoryUUID string            `json:"repository_uuid"` // Repository uuid for associated snapshot
	IsAfter        bool              `json:"is_after"`        // Is the snapshot after the specified date
	Match          *SnapshotResponse `json:"match,omitempty"` // This is the snapshot (if found)
}

type SnapshotCollectionResponse struct {
	Data  []SnapshotResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata   `json:"meta"`  // Metadata about the request
	Links Links              `json:"links"` // Links to other pages of results
}

func (r *SnapshotCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

type RepositoryConfigurationFile string
