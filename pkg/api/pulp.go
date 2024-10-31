package api

type CreateUploadRequest struct {
	Size int64 `json:"size" validate:"required"` // Size of the upload in bytes
}

type PulpUploadChunkRequest struct {
	UploadHref string `param:"upload_href" validate:"required"` // Upload identifier
	File       string `form:"file" validate:"required"`         // A chunk of the uploaded file
	Sha256     string `form:"sha256" validate:"required"`       // SHA-256 checksum of the chunk
}

type FinishUploadRequest struct {
	UploadHref string `param:"upload_href" validate:"required"` // Upload identifier
	Sha256     string `json:"sha256" validate:"required"`       // Expected SHA-256 checksum for the file
}

type TaskRequest struct {
	TaskHref string `param:"task_href" validate:"required"` // Task identifier
}
