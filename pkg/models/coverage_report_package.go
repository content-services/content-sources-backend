package models

import (
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const TableNameCoverageReportPackages = "coverage_report_packages"

const (
	CoverageMatchStatusExact   = "exact"
	CoverageMatchStatusPartial = "partial"
	CoverageMatchStatusNone    = "none"
)

const (
	CoverageMatchSegmentMajor = "major"
	CoverageMatchSegmentMinor = "minor"
	CoverageMatchSegmentPatch = "patch"
	CoverageMatchSegmentNone  = "none"
)

type CoverageReportPackage struct {
	UUID                 string         `json:"uuid" gorm:"primary_key;column:uuid"`
	CoverageReportUUID   string         `json:"coverage_report_uuid" gorm:"not null"`
	Ecosystem            string         `json:"ecosystem" gorm:"not null"`
	Name                 string         `json:"name" gorm:"not null"`
	Version              string         `json:"version" gorm:"not null"`
	Namespace            *string        `json:"namespace,omitempty"`
	MatchStatus          string         `json:"match_status" gorm:"not null"`
	LightwellVersions    pq.StringArray `json:"lightwell_versions" gorm:"type:text[];not null;default:'{}'"`
	NearestHigherVersion string         `json:"nearest_higher_version" gorm:"not null;default:''"`
	MatchSegment         string         `json:"match_segment" gorm:"not null;default:'none'"`

	CveCritical  int      `json:"cve_critical" gorm:"not null;default:0"`
	CveImportant int      `json:"cve_important" gorm:"not null;default:0"`
	CveModerate  int      `json:"cve_moderate" gorm:"not null;default:0"`
	CveLow       int      `json:"cve_low" gorm:"not null;default:0"`
	CveRangeLow  *float32 `json:"cve_range_low,omitempty"`
	CveRangeHigh *float32 `json:"cve_range_high,omitempty"`
}

func (*CoverageReportPackage) TableName() string {
	return TableNameCoverageReportPackages
}

func (p *CoverageReportPackage) BeforeCreate(*gorm.DB) error {
	if p.UUID == "" {
		p.UUID = uuid.NewString()
	}
	if p.LightwellVersions == nil {
		p.LightwellVersions = pq.StringArray{}
	}
	if p.MatchSegment == "" {
		p.MatchSegment = CoverageMatchSegmentNone
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
	if p.MatchStatus == "" {
		return Error{Message: "Match status cannot be blank.", Validation: true}
	}
	if !IsValidCoverageMatchStatus(p.MatchStatus) {
		return Error{Message: "Invalid package match status.", Validation: true}
	}
	if !IsValidCoverageMatchSegment(p.MatchSegment) {
		return Error{Message: "Invalid package match segment.", Validation: true}
	}
	return nil
}

func IsValidCoverageMatchSegment(segment string) bool {
	switch segment {
	case CoverageMatchSegmentMajor, CoverageMatchSegmentMinor, CoverageMatchSegmentPatch, CoverageMatchSegmentNone:
		return true
	default:
		return false
	}
}
