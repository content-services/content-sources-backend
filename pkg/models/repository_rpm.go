package models

import "gorm.io/gorm"

const TableNameRpmsRepositories = "repositories_rpms"

type RepositoryRpm struct {
	RepositoryUUID string `json:"repository_uuid" gorm:"not null"`
	RpmUUID        string `json:"rpm_uuid" gorm:"not null"`
}

func (r *RepositoryRpm) BeforeCreate(db *gorm.DB) (err error) {
	if r.RepositoryUUID == "" {
		return Error{Message: "RepositoryUUID cannot be empty", Validation: true}
	}
	if r.RpmUUID == "" {
		return Error{Message: "RpmUUID cannot be empty", Validation: true}
	}
	return nil
}

func (r *RepositoryRpm) TableName() string {
	return "repositories_rpms"
}
