package seeds

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *SeedSuite) TestSeedCoverageReport() {
	// Count existing demand signals before seeding
	var initialDemandSignalCount int64
	err := s.tx.Model(&models.CoverageDemandSignal{}).Count(&initialDemandSignalCount).Error
	assert.NoError(s.T(), err)

	pendingReport := models.CoverageReport{
		OrgID:  RandomOrgId(),
		Status: config.TaskStatusPending,
	}
	require.NoError(s.T(), s.tx.Create(&pendingReport).Error)

	completedReport, err := SeedCoverageReport(s.tx, CoverageReportSeedOptions{UUID: pendingReport.UUID})
	assert.NoError(s.T(), err)

	var packageCount int64
	err = s.tx.Model(&models.CoverageReportPackage{}).
		Where("coverage_report_uuid = ?", completedReport.UUID).
		Count(&packageCount).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(9), packageCount)

	var demandSignalCount int64
	err = s.tx.Model(&models.CoverageDemandSignal{}).Count(&demandSignalCount).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), initialDemandSignalCount+4, demandSignalCount)

	assert.Equal(s.T(), config.TaskStatusCompleted, completedReport.Status)
	assert.Equal(s.T(), 5, *completedReport.ExactMatches)
	assert.Equal(s.T(), 2, *completedReport.PartialMatches)
	assert.Equal(s.T(), 2, *completedReport.Unmatched)

	// Report-level CVE counts aggregate the per-package counts.
	assert.Equal(s.T(), 2, completedReport.CveCritical)
	assert.Equal(s.T(), 5, completedReport.CveImportant)
	assert.Equal(s.T(), 10, completedReport.CveModerate)
	assert.Equal(s.T(), 21, completedReport.CveLow)

	// Unmatched packages carry no CVE data; matched packages with CVEs get a range.
	var noneCVE models.CoverageReportPackage
	require.NoError(s.T(), s.tx.Where("coverage_report_uuid = ? AND name = ?", completedReport.UUID, "netty-codec-http").First(&noneCVE).Error)
	assert.Zero(s.T(), noneCVE.CveCritical+noneCVE.CveImportant+noneCVE.CveModerate+noneCVE.CveLow)
	assert.Nil(s.T(), noneCVE.CveRangeLow)
	assert.Nil(s.T(), noneCVE.CveRangeHigh)

	var jackson models.CoverageReportPackage
	require.NoError(s.T(), s.tx.Where("coverage_report_uuid = ? AND name = ?", completedReport.UUID, "jackson-databind").First(&jackson).Error)
	assert.Equal(s.T(), 1, jackson.CveCritical)
	require.NotNil(s.T(), jackson.CveRangeHigh)
	assert.Equal(s.T(), float32(9.8), *jackson.CveRangeHigh)
}
