package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameCoverageReports = "coverage_reports"

type EcosystemCoverageSummaryEntry struct {
	Ecosystem      string `json:"ecosystem"`
	Total          int    `json:"total"`
	ExactMatches   int    `json:"exact_matches"`
	PartialMatches int    `json:"partial_matches"`
	Unmatched      int    `json:"unmatched"`
}

type EcosystemCoverageSummary []EcosystemCoverageSummaryEntry

func (s *EcosystemCoverageSummary) Value() (driver.Value, error) {
	if s == nil || len(*s) == 0 {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *EcosystemCoverageSummary) Scan(src interface{}) error {
	if src == nil {
		*s = nil
		return nil
	}
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed")
	}
	var summary EcosystemCoverageSummary
	if err := json.Unmarshal(source, &summary); err != nil {
		return err
	}
	*s = summary
	return nil
}

type CoverageReport struct {
	UUID                     string                    `json:"uuid" gorm:"primary_key;column:uuid"`
	CreatedAt                time.Time                 `json:"created_at" gorm:"not null"`
	OrgID                    string                    `json:"org_id" gorm:"not null"`
	AccountID                *string                   `json:"account_id,omitempty"`
	Status                   string                    `json:"status" gorm:"not null"`
	InputFormat              *string                   `json:"input_format,omitempty"`
	Total                    *int                      `json:"total,omitempty"`
	ExactMatches             *int                      `json:"exact_matches,omitempty"`
	PartialMatches           *int                      `json:"partial_matches,omitempty"`
	Unmatched                *int                      `json:"unmatched,omitempty"`
	EcosystemCoverageSummary *EcosystemCoverageSummary `json:"ecosystem_coverage_summary,omitempty" gorm:"type:jsonb"`
	CatalogSnapshotAt        *time.Time                `json:"catalog_snapshot_at,omitempty"`
	AnalysisTaskError        *string                   `json:"analysis_task_error,omitempty"`
	AnalysisTaskUUID         *string                   `json:"analysis_task_uuid,omitempty"`
	CompletedAt              *time.Time                `json:"completed_at,omitempty"`
}

func (*CoverageReport) TableName() string {
	return TableNameCoverageReports
}

func (cr *CoverageReport) BeforeCreate(*gorm.DB) error {
	if cr.UUID == "" {
		cr.UUID = uuid.NewString()
	}
	if cr.CreatedAt.IsZero() {
		cr.CreatedAt = time.Now()
	}
	return cr.validate()
}

func (cr *CoverageReport) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["status"] = cr.Status
	forUpdate["input_format"] = cr.InputFormat
	forUpdate["total"] = cr.Total
	forUpdate["exact_matches"] = cr.ExactMatches
	forUpdate["partial_matches"] = cr.PartialMatches
	forUpdate["unmatched"] = cr.Unmatched
	forUpdate["ecosystem_coverage_summary"] = cr.EcosystemCoverageSummary
	forUpdate["catalog_snapshot_at"] = cr.CatalogSnapshotAt
	forUpdate["completed_at"] = cr.CompletedAt
	return forUpdate
}

func (cr *CoverageReport) validate() error {
	if cr.OrgID == "" {
		return Error{Message: "Org ID cannot be blank.", Validation: true}
	}
	if cr.Status == "" {
		return Error{Message: "Status cannot be blank.", Validation: true}
	}
	switch cr.Status {
	case config.TaskStatusPending, config.TaskStatusRunning, config.TaskStatusCompleted, config.TaskStatusFailed:
	default:
		return Error{Message: "Invalid coverage report status.", Validation: true}
	}
	return nil
}
