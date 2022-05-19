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
