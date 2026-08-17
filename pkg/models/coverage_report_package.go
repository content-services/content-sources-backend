package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameCoverageReportPackages = "coverage_report_packages"

const (
	CoverageMatchStatusExact   = "exact"
	CoverageMatchStatusPartial = "partial"
	CoverageMatchStatusNone    = "none"
)

type CoverageReportPackage struct {
	UUID               string  `json:"uuid" gorm:"primary_key;column:uuid"`
	CoverageReportUUID string  `json:"coverage_report_uuid" gorm:"not null"`
	Ecosystem          string  `json:"ecosystem" gorm:"not null"`
	Name               string  `json:"name" gorm:"not null"`
	Version            string  `json:"version" gorm:"not null"`
	Namespace          *string `json:"namespace,omitempty"`
	MatchStatus        string  `json:"match_status" gorm:"not null"`
}

func (*CoverageReportPackage) TableName() string {
	return TableNameCoverageReportPackages
}

func (p *CoverageReportPackage) BeforeCreate(*gorm.DB) error {
	if p.UUID == "" {
		p.UUID = uuid.NewString()
	}
	return p.validate()
}

func IsValidCoverageMatchStatus(status string) bool {
	switch status {
	case CoverageMatchStatusExact, CoverageMatchStatusPartial, CoverageMatchStatusNone:
		return true
	default:
		return false
	}
}

func (p *CoverageReportPackage) validate() error {
	if p.CoverageReportUUID == "" {
		return Error{Message: "Coverage report UUID cannot be blank.", Validation: true}
	}
	if p.Ecosystem == "" {
		return Error{Message: "Ecosystem cannot be blank.", Validation: true}
	}
	if p.Name == "" {
		return Error{Message: "Package name cannot be blank.", Validation: true}
	}
	if p.Version == "" {
		return Error{Message: "Version cannot be blank.", Validation: true}
	}
	if p.MatchStatus == "" {
		return Error{Message: "Match status cannot be blank.", Validation: true}
	}
	if !IsValidCoverageMatchStatus(p.MatchStatus) {
		return Error{Message: "Invalid package match status.", Validation: true}
	}
	return nil
}
