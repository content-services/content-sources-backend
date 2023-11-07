package models

import (
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const TableNamePackageGroup = "package_groups"

// RepositoryPackageGroup model for the gorm object of the database
// which represent a package group which belong to one
// repository.
type PackageGroup struct {
	Base
	ID           string         `json:"id" gorm:"not null"`
	Name         string         `json:"name" gorm:"not null"`
	Description  string         `json:"description"`
	PackageList  pq.StringArray `json:"packagelist" gorm:"type:text"`
	Repositories []Repository   `gorm:"many2many:repositories_package_groups"`
}

// BeforeCreate hook performs validations and sets UUID of RepositoryPackageGroup
func (r *PackageGroup) BeforeCreate(tx *gorm.DB) (err error) {
	if err := r.Base.BeforeCreate(tx); err != nil {
		return err
	}
	if r.ID == "" {
		return Error{Message: "ID cannot be empty", Validation: true}
	}
	if r.Name == "" {
		return Error{Message: "Name cannot be empty", Validation: true}
	}
	return nil
}

// DeepCopy clone a RepositoryPackageGroup struct
func (in *PackageGroup) DeepCopy() *PackageGroup {
	out := &PackageGroup{}
	in.DeepCopyInto(out)
	return out
}

func (in *PackageGroup) DeepCopyInto(out *PackageGroup) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)
	out.ID = in.ID
	out.Name = in.Name

	out.Repositories = make([]Repository, len(in.Repositories))
	for i, item := range in.Repositories {
		item.DeepCopyInto(&out.Repositories[i])
	}
}
