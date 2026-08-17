package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameCoverageDemandSignals = "coverage_demand_signals"

const (
	CoverageDemandMatchStatusNone      = "none"
	CoverageDemandMatchStatusPartial   = "partial"
	CoverageDemandSourceProspectDriven = "prospect-driven"
)

type CoverageDemandSignal struct {
	UUID        string    `json:"uuid" gorm:"primary_key;column:uuid"`
	CreatedAt   time.Time `json:"created_at" gorm:"not null"`
	Ecosystem   string    `json:"ecosystem" gorm:"not null"`
	Name        string    `json:"name" gorm:"not null"`
	Version     string    `json:"version" gorm:"not null"`
	Namespace   *string   `json:"namespace,omitempty"`
	MatchStatus string    `json:"match_status" gorm:"not null"`
	Source      string    `json:"source" gorm:"not null"`
}

func (*CoverageDemandSignal) TableName() string {
	return TableNameCoverageDemandSignals
}

func (d *CoverageDemandSignal) BeforeCreate(*gorm.DB) error {
	if d.UUID == "" {
		d.UUID = uuid.NewString()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	return d.validate()
}

func IsValidCoverageDemandMatchStatus(status string) bool {
	switch status {
	case CoverageDemandMatchStatusNone, CoverageDemandMatchStatusPartial:
		return true
	default:
		return false
	}
}

func (d *CoverageDemandSignal) validate() error {
	if d.Ecosystem == "" {
		return Error{Message: "Ecosystem cannot be blank.", Validation: true}
	}
	if d.Name == "" {
		return Error{Message: "Package name cannot be blank.", Validation: true}
	}
	if d.Version == "" {
		return Error{Message: "Version cannot be blank.", Validation: true}
	}
	if d.MatchStatus == "" {
		return Error{Message: "Match status cannot be blank.", Validation: true}
	}
	if !IsValidCoverageDemandMatchStatus(d.MatchStatus) {
		return Error{Message: "Invalid demand signal match status.", Validation: true}
	}
	if d.Source == "" {
		return Error{Message: "Source cannot be blank.", Validation: true}
	}
	if d.Source != CoverageDemandSourceProspectDriven {
		return Error{Message: "Invalid demand signal source.", Validation: true}
	}
	return nil
}
