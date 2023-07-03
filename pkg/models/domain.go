package models

const TableNameDomain = "domains"

// RepositoryRpm model for the gorm object of the database
// which represent a RPM package which belong to one
// repository.
type Domain struct {
	DomainName string `json:"domain_name" gorm:"not null"`
	OrgId      string `json:"org_id" gorm:"not null"`
}
