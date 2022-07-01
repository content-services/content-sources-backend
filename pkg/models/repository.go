package models

import (
	"time"

	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
)

// TODO Review the content for this table.
type Repository struct {
	Base
	URL                      string                    `gorm:"not null"`
	LastReadTime             *time.Time                `gorm:"default:null"`
	LastReadError            *string                   `gorm:"default:null"`
	RepositoryConfigurations []RepositoryConfiguration `gorm:"foreignKey:RepositoryUUID"`
	Rpms                     []Rpm                     `gorm:"many2many:repositories_rpms"`
}

func (r *Repository) BeforeCreate(tx *gorm.DB) (err error) {
	if err := r.Base.BeforeCreate(tx); err != nil {
		return err
	}
	if r.URL == "" {
		return Error{Message: "URL cannot be blank.", Validation: true}
	}
	return nil
}

func (r *Repository) DeepCopy() *Repository {
	var lastReadTime *time.Time = nil
	if r.LastReadTime != nil {
		lastReadTime = &time.Time{}
		*lastReadTime = *r.LastReadTime
	}
	var lastReadError *string = nil
	if r.LastReadError != nil {
		lastReadError = pointy.String(*r.LastReadError)
	}
	item := &Repository{
		Base: Base{
			UUID:      r.UUID,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		},
		URL:                      r.URL,
		LastReadTime:             lastReadTime,
		LastReadError:            lastReadError,
		RepositoryConfigurations: r.RepositoryConfigurations,
		Rpms:                     r.Rpms,
	}
	return item
}

func (r *Repository) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["LastReadTime"] = r.LastReadTime
	forUpdate["LastReadError"] = r.LastReadError

	return forUpdate
}
