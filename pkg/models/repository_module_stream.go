package models

import "gorm.io/gorm"

type RepositoryModuleStream struct {
	RepositoryUUID   string `json:"repository_uuid" gorm:"not null"`
	ModuleStreamUUID string `json:"package_group_uuid" gorm:"not null"`
}

func (r *RepositoryModuleStream) BeforeCreate(db *gorm.DB) (err error) {
	if r.RepositoryUUID == "" {
		return Error{Message: "RepositoryUUID cannot be empty", Validation: true}
	}
	if r.ModuleStreamUUID == "" {
		return Error{Message: "ModuleStreamUUID cannot be empty", Validation: true}
	}
	return nil
}

func (r *RepositoryModuleStream) TableName() string {
	return "repositories_module_streams"
}
