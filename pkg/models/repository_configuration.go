package models

import (
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

func (rc *RepositoryConfiguration) BeforeCreate(tx *gorm.DB) (err error) {
	if err := rc.Base.BeforeCreate(tx); err != nil {
		return err
	}

	if rc.Name == "" {
		err = Error{Message: "Name cannot be blank.", Validation: true}
	}

	if rc.AccountID == "" {
		err = Error{Message: "Account ID cannot be blank.", Validation: true}
	}

	if rc.OrgID == "" {
		err = Error{Message: "Org ID cannot be blank.", Validation: true}
	}

	if rc.RepositoryUUID == "" {
		err = Error{Message: "Repository UUID foreign key cannot be blank.", Validation: true}
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
