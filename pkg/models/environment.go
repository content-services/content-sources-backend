package models

import (
	"gorm.io/gorm"
)

const TableNameEnvironment = "environments"

// RepositoryEnvironment model for the gorm object of the database
// which represents an environment which belongs to one
// repository.
type Environment struct {
	Base
	ID           string       `json:"id" gorm:"not null"`
	Name         string       `json:"name" gorm:"not null"`
	Description  string       `json:"description"`
	Repositories []Repository `gorm:"many2many:repositories_environments"`
}

// BeforeCreate hook performs validations and sets UUID of RepositoryEnvironment
func (r *Environment) BeforeCreate(tx *gorm.DB) (err error) {
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

// DeepCopy clone a RepositoryEnvironment struct
func (in *Environment) DeepCopy() *Environment {
	out := &Environment{}
	in.DeepCopyInto(out)
	return out
}

func (in *Environment) DeepCopyInto(out *Environment) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)
	out.ID = in.ID
	out.Name = in.Name
	out.Description = in.Description

	out.Repositories = make([]Repository, len(in.Repositories))
	for i, item := range in.Repositories {
		item.DeepCopyInto(&out.Repositories[i])
	}
}
