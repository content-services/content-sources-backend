package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameCoverageUploads = "coverage_uploads"

type CoverageUpload struct {
	UUID       string    `json:"uuid" gorm:"primary_key;column:uuid"`
	StorageKey string    `json:"storage_key" gorm:"not null"`
	Sha256     string    `json:"sha256" gorm:"not null"`
	SizeBytes  int64     `json:"size_bytes" gorm:"not null"`
	CreatedAt  time.Time `json:"created_at" gorm:"not null"`
}

func (*CoverageUpload) TableName() string {
	return TableNameCoverageUploads
}

func (u *CoverageUpload) BeforeCreate(*gorm.DB) error {
	if u.UUID == "" {
		u.UUID = uuid.NewString()
	}
	if u.StorageKey == "" {
		u.StorageKey = "coverage-uploads/" + u.UUID
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	return u.validate()
}

func (u *CoverageUpload) validate() error {
	if u.StorageKey == "" {
		return Error{Message: "Storage key cannot be blank.", Validation: true}
	}
	if u.Sha256 == "" {
		return Error{Message: "Sha256 cannot be blank.", Validation: true}
	}
	if u.SizeBytes <= 0 {
		return Error{Message: "Size bytes must be greater than zero.", Validation: true}
	}
	return nil
}
