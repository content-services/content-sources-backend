package models

import (
	"gorm.io/gorm"
)

const TableNameRpmsRepositories = "repositories_rpms"

// RepositoryRpm model for the gorm object of the database
// which represent a RPM package which belong to one
// repository.
type Rpm struct {
	Base
	Name    string `json:"name" gorm:"not null"`
	Arch    string `json:"arch" gorm:"not null"`
	Version string `json:"version" gorm:"not null"`
	Release string `json:"release" gorm:"null"`
	// Epoch is a way to define weighted dependencies based
	// on version numbers. It's default value is 0 and this
	// is assumed if an Epoch directive is not listed in the RPM SPEC file.
	// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/packaging_and_distributing_software/advanced-topics#packaging-epoch_epoch-scriplets-and-triggers
	Epoch       int32  `json:"epoch" gorm:"default:0;not null"`
	Summary     string `json:"summary" gorm:"not null"`
	Description string `json:"description" gorm:"not null"`
	Checksum    string `json:"checksum" gorm:"not null"`
	// RepositoryUUID string     `gorm:"not null"`
	// Repository     Repository `gorm:"foreignKey:UUID;references:RepositoriesUUID"`
	Repositories []Repository `gorm:"many2many:repositories_rpms"`
}

// BeforeCreate hook for ReposirotyRpm records
// Return error if any else nil
func (r *Rpm) BeforeCreate(tx *gorm.DB) (err error) {
	if err := r.Base.BeforeCreate(tx); err != nil {
		return err
	}
	return nil
}

// DeepCopy clone a RepositoryRpm struct
func (r *Rpm) DeepCopy() *Rpm {
	return &Rpm{
		Base: Base{
			UUID:      r.UUID,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		},
		Name:        r.Name,
		Arch:        r.Arch,
		Version:     r.Version,
		Release:     r.Release,
		Epoch:       r.Epoch,
		Summary:     r.Summary,
		Description: r.Description,
	}
}
