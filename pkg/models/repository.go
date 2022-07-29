package models

import (
	"time"

	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
)

// https://stackoverflow.com/questions/43587610/preventing-null-or-empty-string-values-in-the-db
// TODO Review the content for this table.
type Repository struct {
	Base
	URL           string     `gorm:"unique;not null;default:null"`
	LastReadTime  *time.Time `gorm:"default:null"`
	LastReadError *string    `gorm:"default:null"`
	Public        bool

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

func (in *Repository) DeepCopy() *Repository {
	out := &Repository{}
	in.DeepCopyInto(out)
	return out
}

func (in *Repository) DeepCopyInto(out *Repository) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)
	var lastReadTime *time.Time = nil
	if in.LastReadTime != nil {
		lastReadTime = &time.Time{}
		*lastReadTime = *in.LastReadTime
	}
	var lastReadError *string = nil
	if in.LastReadError != nil {
		lastReadError = pointy.String(*in.LastReadError)
	}
	out.URL = in.URL
	out.LastReadTime = lastReadTime
	out.LastReadError = lastReadError
	out.Public = in.Public

	// Duplicate the slices
	out.RepositoryConfigurations = make([]RepositoryConfiguration, len(in.RepositoryConfigurations))
	for i, item := range in.RepositoryConfigurations {
		item.DeepCopyInto(&out.RepositoryConfigurations[i])
	}
	out.Rpms = make([]Rpm, len(in.Rpms))
	for i, item := range in.Rpms {
		item.DeepCopyInto(&out.Rpms[i])
	}
}

func (r *Repository) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["LastReadTime"] = r.LastReadTime
	forUpdate["LastReadError"] = r.LastReadError
	forUpdate["URL"] = r.URL
	forUpdate["Public"] = r.Public

	return forUpdate
}
