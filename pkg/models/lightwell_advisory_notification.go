package models

import "gorm.io/gorm"

type LightwellAdvisoryNotification struct {
	Base
	RepositoryConfigurationUUID string `json:"repository_configuration_uuid" gorm:"not null"`
	AdvisoryID                  string `json:"advisory_id" gorm:"not null"`
	PackageName                 string `json:"package_name"`
	OrgID                       string `json:"org_id" gorm:"not null;default:''"`
}

func (*LightwellAdvisoryNotification) TableName() string {
	return "lightwell_advisory_notifications"
}

func (lan *LightwellAdvisoryNotification) BeforeCreate(tx *gorm.DB) error {
	if err := lan.Base.BeforeCreate(tx); err != nil {
		return err
	}
	return lan.validate()
}

func (lan *LightwellAdvisoryNotification) validate() error {
	if lan.RepositoryConfigurationUUID == "" {
		return Error{Message: "Repository configuration UUID cannot be blank.", Validation: true}
	}
	if lan.AdvisoryID == "" {
		return Error{Message: "Advisory ID cannot be blank.", Validation: true}
	}
	return nil
}
