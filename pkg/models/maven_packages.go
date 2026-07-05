package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameMavenPackages = "maven_packages"

type MavenPackage struct {
	UUID       string  `json:"uuid" gorm:"primary_key;column:uuid"`
	Name       string  `json:"name" gorm:"column:name;not null"`
	Summary    *string `json:"summary,omitempty" gorm:"column:summary"`
	License    *string `json:"license,omitempty" gorm:"column:license"`
	ProjectURL *string `json:"project_url,omitempty" gorm:"column:project_url"`
	Author     *string `json:"author,omitempty" gorm:"column:author"`
}

func (m *MavenPackage) BeforeCreate(db *gorm.DB) (err error) {
	if m.UUID == "" {
		m.UUID = uuid.NewString()
	}
	return nil
}

func (*MavenPackage) TableName() string {
	return TableNameMavenPackages
}
