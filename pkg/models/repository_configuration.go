package models

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type RepositoryConfiguration struct {
	Base
	Name           string         `json:"name" gorm:"default:null"`
	Versions       pq.StringArray `json:"version" gorm:"type:text[],default:null"`
	Arch           string         `json:"arch" gorm:"default:''"`
	AccountID      string         `json:"account_id" gorm:"default:null"`
	OrgID          string         `json:"org_id" gorm:"default:null"`
	RepositoryUUID string         `json:"repository_uuid" gorm:"not null"`
	Repository     Repository     `json:"repository,omitempty"`
}

// When updating a model with gorm, we want to explicitly update any field that is set to
// empty string.  We always fetch the object and then update it before saving
// so every update is the full model of user changeable fields.
// So OrgId and account Id are excluded
func (rc *RepositoryConfiguration) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["Name"] = rc.Name
	forUpdate["Arch"] = rc.Arch
	forUpdate["Versions"] = rc.Versions
	forUpdate["AccountID"] = rc.AccountID
	forUpdate["OrgID"] = rc.OrgID
	forUpdate["RepositoryUUID"] = rc.RepositoryUUID

	return forUpdate
}

// BeforeCreate perform validations and sets UUID of Repository Configurations
func (rc *RepositoryConfiguration) BeforeCreate(tx *gorm.DB) error {
	if err := rc.Base.BeforeCreate(tx); err != nil {
		return err
	}
	if err := rc.DedupeVersions(tx); err != nil {
		return err
	}
	if err := rc.validate(); err != nil {
		return err
	}
	return nil
}

// BeforeUpdate perform validations of Repository Configurations
func (rc *RepositoryConfiguration) BeforeUpdate(tx *gorm.DB) error {
	if err := rc.DedupeVersions(tx); err != nil {
		return err
	}
	if err := rc.validate(); err != nil {
		return err
	}
	return nil
}

func (rc *RepositoryConfiguration) DedupeVersions(tx *gorm.DB) error {
	var versionMap = make(map[string]bool)
	var unique pq.StringArray
	for i := 0; i < len(rc.Versions); i++ {
		if _, found := versionMap[rc.Versions[i]]; !found {
			versionMap[rc.Versions[i]] = true
			unique = append(unique, rc.Versions[i])
		}
	}
	tx.Statement.SetColumn("Versions", unique)
	return nil
}

func (rc *RepositoryConfiguration) validate() error {
	var err error
	if rc.Name == "" {
		err = Error{Message: "Name cannot be blank.", Validation: true}
		return err
	}

	if rc.OrgID == "" {
		err = Error{Message: "Org ID cannot be blank.", Validation: true}
		return err
	}

	if rc.RepositoryUUID == "" {
		err = Error{Message: "Repository UUID foreign key cannot be blank.", Validation: true}
		return err
	}

	if rc.Arch != "" && !config.ValidArchLabel(rc.Arch) {
		return Error{Message: fmt.Sprintf("Specified distribution architecture %s is invalid.", rc.Arch),
			Validation: true}
	}
	valid, invalidVer := config.ValidDistributionVersionLabels(rc.Versions)
	if len(rc.Versions) > 0 && !valid {
		return Error{Message: fmt.Sprintf("Specified distribution version %s is invalid.", invalidVer),
			Validation: true}
	}
	return nil
}

func (in *RepositoryConfiguration) DeepCopyInto(out *RepositoryConfiguration) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)
	out.Name = in.Name
	out.Versions = in.Versions
	out.Arch = in.Arch
	out.AccountID = in.AccountID
	out.OrgID = in.OrgID
	out.RepositoryUUID = in.RepositoryUUID
}

func (in *RepositoryConfiguration) DeepCopy() *RepositoryConfiguration {
	var out = &RepositoryConfiguration{}
	in.DeepCopyInto(out)
	return out
}
