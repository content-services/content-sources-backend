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

func (in *Base) DeepCopy() *Base {
	out := &Base{}
	in.DeepCopyInto(out)
	return out
}

func (in *Base) DeepCopyInto(out *Base) {
	if in == nil || out == nil || in == out {
		return
	}
	out.UUID = in.UUID
	out.CreatedAt = in.CreatedAt
	out.UpdatedAt = in.UpdatedAt
}

func DropAll(db *gorm.DB) error {
	result := db.Where("1=1").Delete(Rpm{})
	if result.Error != nil {
		return result.Error
	}
	result = db.Where("1=1").Delete(RepositoryConfiguration{})
	if result.Error != nil {
		return result.Error
	}
	result = db.Where("1=1").Delete(Repository{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}
