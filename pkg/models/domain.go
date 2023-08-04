package models

const TableNameDomain = "domains"

// Domain model keeps track of the mapping between org id and the generated domain name
type Domain struct {
	DomainName string `json:"domain_name" gorm:"not null"`
	OrgId      string `json:"org_id" gorm:"not null"`
}
