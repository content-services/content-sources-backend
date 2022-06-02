package models

type RepositoryConfiguration struct {
	Base
	Name      string `json:"name" gorm:"default:null"`
	URL       string `json:"url" gorm:"default:null"`
	Version   string `json:"version" gorm:"default:null"`
	Arch      string `json:"arch" gorm:"default:null"`
	AccountID string `json:"account_id" gorm:"default:null"`
	OrgID     string `json:"org_id" gorm:"default:null"`
}

//When updating a model with gorm, we want to explicitly update any field that is set to
// empty string.  We always fetch the object and then update it before saving
// so every update is the full model of user changeable fields.
// So OrgId and account Id are excluded
func (rc *RepositoryConfiguration) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["Name"] = rc.Name
	forUpdate["URL"] = rc.URL
	forUpdate["Arch"] = rc.Arch
	forUpdate["Version"] = rc.Version
	return forUpdate
}
