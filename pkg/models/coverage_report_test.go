package models

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CoverageReportSuite struct {
	*ModelsSuite
}

func TestCoverageReportSuite(t *testing.T) {
	m := ModelsSuite{}
	r := CoverageReportSuite{&m}
	suite.Run(t, &r)
}

func (s *CoverageReportSuite) TestCoverageReportCreate() {
	tx := s.tx

	createdAt := time.Now().Truncate(time.Microsecond)
	completedAt := createdAt.Add(60 * time.Second)
	catalogSnapshotAt := createdAt.Add(30 * time.Second)
	taskUUID := uuid.New().String()

	report := CoverageReport{
		OrgID:          "12345",
		AccountID:      utils.Ptr("account-1"),
		Status:         config.TaskStatusCompleted,
		CreatedAt:      createdAt,
		InputFormat:    utils.Ptr("cyclonedx"),
		Total:          utils.Ptr(15),
		ExactMatches:   utils.Ptr(8),
		PartialMatches: utils.Ptr(3),
		Unmatched:      utils.Ptr(4),
		EcosystemCoverageSummary: &EcosystemCoverageSummary{
			{Ecosystem: "Java", Total: 9, ExactMatches: 4, PartialMatches: 2, Unmatched: 3},
			{Ecosystem: "Python", Total: 6, ExactMatches: 4, PartialMatches: 1, Unmatched: 1},
		},
		CatalogSnapshotAt: utils.Ptr(catalogSnapshotAt),
		AnalysisTaskUUID:  utils.Ptr(taskUUID),
		CompletedAt:       utils.Ptr(completedAt),
	}
	err := tx.Create(&report).Error
	assert.NoError(s.T(), err)

	readReport := CoverageReport{}
	err = tx.Where("uuid = ?", report.UUID).First(&readReport).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), report.UUID, readReport.UUID)
	assert.Equal(s.T(), report.OrgID, readReport.OrgID)
	assert.Equal(s.T(), *report.AccountID, *readReport.AccountID)
	assert.Equal(s.T(), report.Status, readReport.Status)
	assert.Equal(s.T(), report.CreatedAt, readReport.CreatedAt)
	assert.Equal(s.T(), *report.InputFormat, *readReport.InputFormat)
	assert.Equal(s.T(), *report.Total, *readReport.Total)
	assert.Equal(s.T(), *report.ExactMatches, *readReport.ExactMatches)
	assert.Equal(s.T(), *report.PartialMatches, *readReport.PartialMatches)
	assert.Equal(s.T(), *report.Unmatched, *readReport.Unmatched)
	assert.Nil(s.T(), readReport.AnalysisTaskError)
	summary := *readReport.EcosystemCoverageSummary
	assert.Len(s.T(), summary, 2)
	assert.Equal(s.T(), "Java", summary[0].Ecosystem)
	assert.Equal(s.T(), 9, summary[0].Total)
	assert.Equal(s.T(), "Python", summary[1].Ecosystem)
	assert.Equal(s.T(), 6, summary[1].Total)
	assert.Equal(s.T(), report.CatalogSnapshotAt, readReport.CatalogSnapshotAt)
	assert.Equal(s.T(), *report.AnalysisTaskUUID, *readReport.AnalysisTaskUUID)
	assert.Equal(s.T(), report.CompletedAt, readReport.CompletedAt)
}

func (s *CoverageReportSuite) TestCoverageReportValidations() {
	t := s.T()
	tx := s.tx

	spName := "testcoveragereportvalidations"
	testOrgID := "12345"
	testStatus := config.TaskStatusCompleted

	testCases := []struct {
		given    CoverageReport
		expected string
	}{
		{
			given: CoverageReport{
				OrgID:  "",
				Status: testStatus,
			},
			expected: "Org ID cannot be blank.",
		},
		{
			given: CoverageReport{
				OrgID:  testOrgID,
				Status: "",
			},
			expected: "Status cannot be blank.",
		},
		{
			given: CoverageReport{
				OrgID:  testOrgID,
				Status: "invalid",
			},
			expected: "Invalid coverage report status.",
		},
	}

	tx.SavePoint(spName)
	for _, item := range testCases {
		err := tx.Create(&item.given).Error
		assert.Error(t, err)
		if err != nil {
			assert.Equal(t, item.expected, err.Error())
		}
		tx.RollbackTo(spName)
	}
}
