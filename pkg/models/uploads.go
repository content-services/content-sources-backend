package models

import (
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Upload struct {
	UploadUUID    string
	CreatedAt     time.Time
	OrgID         string
	ChunkSize     int64
	Size          int64
	Sha256        string
	ChunkList     pq.StringArray      `gorm:"type:text[]"`
	ChunkMetadata UploadChunkMetadata `gorm:"serializer:json;type:jsonb;not null;default:'{}'"`
}

type UploadChunkMetadataEntry struct {
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}

// Keys are the starting offsets (start of the content range)
type UploadChunkMetadata map[string]UploadChunkMetadataEntry

// BeforeCreate perform validations and sets UUID of Upload
func (t *Upload) BeforeCreate(tx *gorm.DB) error {
	if err := t.validate(); err != nil {
		return err
	}

	if t.ChunkMetadata == nil {
		t.ChunkMetadata = UploadChunkMetadata{}
	}

	return nil
}

func (t *Upload) validate() error {
	var err error
	if t.UploadUUID == "" {
		err = Error{Message: "Upload UUID cannot be blank.", Validation: true}
		return err
	}

	if t.OrgID == "" {
		err = Error{Message: "Org ID cannot be blank.", Validation: true}
		return err
	}

	if t.ChunkSize == 0 {
		err = Error{Message: "ChunkSize cannot be 0.", Validation: true}
		return err
	}

	if t.Size == 0 {
		err = Error{Message: "Size cannot be 0.", Validation: true}
		return err
	}

	if t.Sha256 == "" {
		err = Error{Message: "Sha256 cannot be blank.", Validation: true}
		return err
	}

	return nil
}
