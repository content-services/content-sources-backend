package models

import "gorm.io/gorm"

const TableNameEnvironmentsRepositories = "repositories_environments"

type RepositoryEnvironment struct {
	RepositoryUUID  string `json:"repository_uuid" gorm:"not null"`
	EnvironmentUUID string `json:"environment_uuid" gorm:"not null"`
}

func (r *RepositoryEnvironment) BeforeCreate(db *gorm.DB) (err error) {
	if r.RepositoryUUID == "" {
		return Error{Message: "RepositoryUUID cannot be empty", Validation: true}
	}
	if r.EnvironmentUUID == "" {
		return Error{Message: "EnvironmentUUID cannot be empty", Validation: true}
	}
	return nil
}

func (r *RepositoryEnvironment) TableName() string {
	return "repositories_environments"
}
