package models

import (
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"gorm.io/gorm"
)

type Template struct {
	Base
	Name                     string         `gorm:"not null;default:null"`
	OrgID                    string         `gorm:"default:null"`
	Description              string         `gorm:"default:null"`
	Date                     time.Time      `gorm:"default:null"`
	Version                  string         `gorm:"default:null"`
	Arch                     string         `gorm:"default:null"`
	DeletedAt                gorm.DeletedAt `json:"deleted_at"`
	CreatedBy                string
	LastUpdatedBy            string
	UseLatest                bool
	RHSMEnvironmentCreated   bool                      `json:"rhsm_environment_created" gorm:"column:rhsm_environment_created"`
	LastUpdateSnapshotError  *string                   `gorm:"default:null"`
	LastUpdateTaskUUID       string                    `json:"last_update_task_uuid" gorm:"default:null"`
	LastUpdateTask           *TaskInfo                 `json:"last_update_task" gorm:"foreignKey:last_update_task_uuid"`
	RepositoryConfigurations []RepositoryConfiguration `gorm:"many2many:templates_repository_configurations"`
}

// BeforeCreate perform validations and sets UUID of Template
func (t *Template) BeforeCreate(tx *gorm.DB) error {
	if err := t.Base.BeforeCreate(tx); err != nil {
		return err
	}
	if err := t.validate(); err != nil {
		return err
	}
	return nil
}

// BeforeUpdate perform validations and sets UUID of Template
func (t *Template) BeforeUpdate(tx *gorm.DB) error {
	if err := t.validate(); err != nil {
		return err
	}
	return nil
}

func (t *Template) AfterFind(tx *gorm.DB) error {
	if err := t.Base.AfterFind(tx); err != nil {
		return err
	}
	t.DeletedAt = gorm.DeletedAt{Time: t.DeletedAt.Time.UTC()}
	t.Date = t.Date.UTC()
	return nil
}

func (t *Template) AfterSave(tx *gorm.DB) error {
	return t.AfterFind(tx)
}

func (t *Template) validate() error {
	var err error
	if t.Name == "" {
		err = Error{Message: "Name cannot be blank.", Validation: true}
		return err
	}

	if t.OrgID == "" {
		err = Error{Message: "Org ID cannot be blank.", Validation: true}
		return err
	}

	if t.Arch == "" {
		err = Error{Message: "Arch cannot be blank.", Validation: true}
		return err
	}

	if t.Version == "" {
		err = Error{Message: "Version cannot be blank.", Validation: true}
		return err
	}

	if t.Arch != "" && !config.ValidArchLabel(t.Arch) {
		return Error{Message: fmt.Sprintf("Specified architecture %s is invalid.", t.Arch),
			Validation: true}
	}
	valid, invalidVer := config.ValidDistributionVersionLabels([]string{t.Version})
	if len(t.Version) > 0 && !valid {
		return Error{Message: fmt.Sprintf("Specified distribution version %s is invalid.", invalidVer),
			Validation: true}
	}

	if t.UseLatest && !t.Date.IsZero() {
		err = Error{Message: "Date must be null if use_latest is true.", Validation: true}
		return err
	}

	return nil
}

func (t *Template) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	// Version and arch cannot be updated
	forUpdate["description"] = t.Description
	forUpdate["date"] = t.Date
	forUpdate["last_updated_by"] = t.LastUpdatedBy
	forUpdate["name"] = t.Name
	forUpdate["use_latest"] = t.UseLatest
	return forUpdate
}
