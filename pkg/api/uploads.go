package api

import "time"

type AddUploadsRequest struct {
	Uploads   []Upload   `json:"uploads"`   // List of unfinished uploads
	Artifacts []Artifact `json:"artifacts"` // List of created artifacts
}

type Upload struct {
	Uuid   string `json:"uuid"`   // Upload UUID, use with public API
	Href   string `json:"href"`   // HREF to the unfinished upload, use with internal API
	Sha256 string `json:"sha256"` // SHA256 sum of the uploaded file
}

type Artifact struct {
	Href   string // HREF to the  completed artifact
	Sha256 string // SHA256 sum of the completed artifact
}

type PublicUploadChunkRequest struct {
	UploadUuid string `param:"upload_uuid"` // Upload UUID
	File       string `form:"file"`         // A chunk of the uploaded file
	Sha256     string `form:"sha256"`       // SHA-256 checksum of the chunk
}

type UploadResponse struct {
	UploadUuid  *string    `json:"upload_uuid"`         // Upload UUID
	Created     *time.Time `json:"created"`             // Timestamp of creation
	LastUpdated *time.Time `json:"last_updated"`        // Timestamp of last update
	Size        int64      `json:"size"`                // Size of the upload in bytes
	Completed   *time.Time `json:"completed,omitempty"` // Timestamp when upload is committed
}
