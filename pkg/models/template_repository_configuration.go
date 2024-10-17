package models

import (
	"gorm.io/gorm"
)

const TableNameTemplatesRepositoryConfigurations = "templates_repository_configurations"

type TemplateRepositoryConfiguration struct {
	RepositoryConfigurationUUID string         `json:"repository_configuration_uuid" gorm:"not null"`
	TemplateUUID                string         `json:"template_uuid" gorm:"not null"`
	SnapshotUUID                string         `json:"snapshot_uuid" gorm:"not null"`
	DistributionHref            string         `json:"distribution_href"`
	DeletedAt                   gorm.DeletedAt `json:"deleted_at"`
}

func (t *TemplateRepositoryConfiguration) BeforeCreate(db *gorm.DB) (err error) {
	if t.RepositoryConfigurationUUID == "" {
		return Error{Message: "RepositoryConfigurationUUID cannot be empty", Validation: true}
	}
	if t.TemplateUUID == "" {
		return Error{Message: "TemplateUUID cannot be empty", Validation: true}
	}
	return nil
}

func (t *TemplateRepositoryConfiguration) AfterFind(tx *gorm.DB) error {
	t.DeletedAt = gorm.DeletedAt{
		Time:  t.DeletedAt.Time.UTC(),
		Valid: t.DeletedAt.Valid,
	}
	return nil
}

func (t *TemplateRepositoryConfiguration) AfterSave(tx *gorm.DB) error {
	return t.AfterFind(tx)
}

func (t *TemplateRepositoryConfiguration) TableName() string {
	return TableNameTemplatesRepositoryConfigurations
}
