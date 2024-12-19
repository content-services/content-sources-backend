package models

import (
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Upload struct {
	UploadUUID string
	CreatedAt  time.Time
	OrgID      string
	ChunkSize  int64
	Sha256     string
	ChunkList  pq.StringArray `gorm:"type:text[]"`
}

// BeforeCreate perform validations and sets UUID of Upload
func (t *Upload) BeforeCreate(tx *gorm.DB) error {
	if err := t.validate(); err != nil {
		return err
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

	if t.Sha256 == "" {
		err = Error{Message: "Sha256 cannot be blank.", Validation: true}
		return err
	}

	return nil
}
