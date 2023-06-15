package api

import "time"

type SnapshotResponse struct {
	CreatedAt        time.Time        `json:"created_at"`        // Datetime the snapshot was created
	DistributionPath string           `json:"distribution_path"` // Path to pulp distribution
	ContentCounts    map[string]int64 `json:"content_counts"`    // Count of each content type

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
