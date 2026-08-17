package seeds

import (
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

func (s *SeedSuite) TestSeedCoverageReport() {
	report, err := SeedCoverageReport(s.tx, CoverageReportSeedOptions{OrgID: RandomOrgId()})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), stubCompletedReportUUID, report.UUID)

	var packageCount int64
	err = s.tx.Model(&models.CoverageReportPackage{}).
		Where("coverage_report_uuid = ?", report.UUID).
		Count(&packageCount).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(15), packageCount)

	var demandSignalCount int64
	err = s.tx.Model(&models.CoverageDemandSignal{}).Count(&demandSignalCount).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(9), demandSignalCount)

	var highReport models.CoverageReport
	err = s.tx.Where("uuid = ?", stubHighCoverageReportUUID).First(&highReport).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 8, *highReport.ExactMatches)
	assert.Equal(s.T(), 1, *highReport.PartialMatches)
	assert.Equal(s.T(), 1, *highReport.Unmatched)
}
