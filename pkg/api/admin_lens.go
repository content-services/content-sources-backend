package api

import "time"

type AdminDemandSignalFilterData struct {
	Ecosystem         string     // Only include signals for this ecosystem
	BeforeCreatedDate *time.Time // Only include signals created strictly before this time
	AfterCreatedDate  *time.Time // Only include signals created at or after this time
}

type AdminDemandSignalResponse struct {
	Count     int64   `json:"count"`               // Number of demand signals for this package version
	Name      string  `json:"name"`                // Package name
	Version   string  `json:"version"`             // Package version
	Ecosystem string  `json:"ecosystem"`           // Package ecosystem
	Namespace *string `json:"namespace,omitempty"` // Package namespace (e.g. Maven groupId)
}

type AdminDemandSignalCollectionResponse struct {
	Data  []AdminDemandSignalResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata            `json:"meta"`  // Metadata about the request
	Links Links                       `json:"links"` // Links to other pages of results
}

func (r *AdminDemandSignalCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
