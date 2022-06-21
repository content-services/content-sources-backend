package models

import (
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type RepositoryConfiguration struct {
	Base
	Name      string         `json:"name" gorm:"default:null"`
	URL       string         `json:"url" gorm:"default:null"`
	Versions  pq.StringArray `json:"version" gorm:"type:text[],default:null"`
	Arch      string         `json:"arch" gorm:"default:''"`
	AccountID string         `json:"account_id" gorm:"default:null"`
	OrgID     string         `json:"org_id" gorm:"default:null"`
}

// When updating a model with gorm, we want to explicitly update any field that is set to
// empty string.  We always fetch the object and then update it before saving
// so every update is the full model of user changeable fields.
// So OrgId and account Id are excluded
func (rc *RepositoryConfiguration) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["Name"] = rc.Name
	forUpdate["URL"] = rc.URL
	forUpdate["Arch"] = rc.Arch
	forUpdate["Versions"] = rc.Versions
	return forUpdate
}

func (rc *RepositoryConfiguration) BeforeCreate(tx *gorm.DB) (err error) {
	rc.Base.UUID = uuid.NewString()

	if rc.Name == "" {
		err = Error{Message: "Name cannot be blank.", Validation: true}
	}
	if rc.URL == "" {
		err = Error{Message: "URL cannot be blank.", Validation: true}
	}
	if rc.AccountID == "" {
		err = Error{Message: "Account ID cannot be blank.", Validation: true}
	}
	if rc.OrgID == "" {
		err = Error{Message: "Org ID cannot be blank.", Validation: true}
	}
	return
}
