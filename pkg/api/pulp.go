package api

type CreateUploadRequest struct {
	Size int64 `json:"size"` // Size of the upload in bytes
}

type UploadChunkRequest struct {
	UploadHref string `param:"upload_href"` // Upload identifier
	File       string `form:"file"`         // A chunk of the uploaded file
	Sha256     string `form:"sha256"`       // SHA-256 checksum of the chunk
}

type FinishUploadRequest struct {
	UploadHref string `param:"upload_href"` // Upload identifier
	Sha256     string `json:"sha256"`       // Expected SHA-256 checksum for the file
}

type TaskRequest struct {
	TaskHref string `param:"task_href"` // Task identifier
}
