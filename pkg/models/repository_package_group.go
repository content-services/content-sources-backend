package models

import "gorm.io/gorm"

const TableNamePackageGroupsRepositories = "repositories_package_groups"

type RepositoryPackageGroup struct {
	RepositoryUUID   string `json:"repository_uuid" gorm:"not null"`
	PackageGroupUUID string `json:"package_group_uuid" gorm:"not null"`
}

func (r *RepositoryPackageGroup) BeforeCreate(db *gorm.DB) (err error) {
	if r.RepositoryUUID == "" {
		return Error{Message: "RepositoryUUID cannot be empty", Validation: true}
	}
	if r.PackageGroupUUID == "" {
		return Error{Message: "PackageGroupUUID cannot be empty", Validation: true}
	}
	return nil
}

func (r *RepositoryPackageGroup) TableName() string {
	return "repositories_package_groups"
}
