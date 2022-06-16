package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Base struct {
	UUID      string `gorm:"primary_key" json:"uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (base *Base) BeforeCreate(db *gorm.DB) (err error) {
	base.UUID = uuid.NewString()
	return
}

type Error struct {
	Message    string
	Validation bool
}

func (e Error) Error() string {
	return e.Message
}
