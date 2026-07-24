package models

import (
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type LightwellAdvisory struct {
	Base
	RepoName                    string         `json:"repo_name" gorm:"not null"`
	AdvisoryID                  string         `json:"advisory_id" gorm:"not null"`
	Severity                    string         `json:"severity" gorm:"type:varchar(255)"`
	Details                     string         `json:"details"`
	ReferenceURLs               pq.StringArray `json:"reference_urls" gorm:"type:text[]"`
	PackageName                 string         `json:"package_name"`
	FixedVersion                string         `json:"fixed_version"`
	RepositoryConfigurationUUID string         `json:"repository_configuration_uuid" gorm:"not null"`
	Checksum                    string         `json:"checksum"`
}

func (*LightwellAdvisory) TableName() string {
	return "lightwell_advisories"
}

func (la *LightwellAdvisory) BeforeCreate(tx *gorm.DB) error {
	if err := la.Base.BeforeCreate(tx); err != nil {
		return err
	}
	return la.validate()
}

func (la *LightwellAdvisory) BeforeUpdate(tx *gorm.DB) error {
	return la.validate()
}

func (la *LightwellAdvisory) validate() error {
	if la.RepoName == "" {
		return Error{Message: "Repo name cannot be blank.", Validation: true}
	}
	if la.AdvisoryID == "" {
		return Error{Message: "Advisory ID cannot be blank.", Validation: true}
	}
	if la.RepositoryConfigurationUUID == "" {
		return Error{Message: "Repository configuration UUID cannot be blank.", Validation: true}
	}
	if la.Checksum == "" {
		return Error{Message: "Checksum cannot be blank.", Validation: true}
	}
	return nil
}
