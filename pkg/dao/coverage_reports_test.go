package dao

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CoverageReportDaoSuite struct {
	*DaoSuite
}

func TestCoverageReportDaoSuite(t *testing.T) {
	m := DaoSuite{}
	r := CoverageReportDaoSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *CoverageReportDaoSuite) dao() coverageReportDaoImpl {
	return coverageReportDaoImpl{db: s.tx}
}

func (s *CoverageReportDaoSuite) createReport(orgID string, status string) models.CoverageReport {
	report := models.CoverageReport{
		OrgID:  orgID,
		Status: status,
	}
	err := s.tx.Create(&report).Error
	require.NoError(s.T(), err)
	return report
}

func (s *CoverageReportDaoSuite) createUpload() models.CoverageUpload {
	upload := models.CoverageUpload{
		StorageKey: "storage/key",
		Sha256:     "testsha",
		SizeBytes:  1024,
	}
	err := s.tx.Create(&upload).Error
	require.NoError(s.T(), err)
	return upload
}

func (s *CoverageReportDaoSuite) createPackage(reportUUID string, name string, version string, ecosystem string, matchStatus string) models.CoverageReportPackage {
	pkg := models.CoverageReportPackage{
		CoverageReportUUID: reportUUID,
		Name:               name,
		Version:            version,
		Ecosystem:          ecosystem,
		MatchStatus:        matchStatus,
	}
	err := s.tx.Create(&pkg).Error
	require.NoError(s.T(), err)
	return pkg
}

func (s *CoverageReportDaoSuite) TestFetch() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)
	report.Total = utils.Ptr(10)
	report.ExactMatches = utils.Ptr(8)
	report.PartialMatches = utils.Ptr(1)
	report.Unmatched = utils.Ptr(1)
	report.InputFormat = utils.Ptr("CycloneDX")
	require.NoError(s.T(), s.tx.Save(&report).Error)

	resp, err := s.dao().Fetch(context.Background(), orgID, report.UUID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), report.UUID, resp.UUID)
	assert.Equal(s.T(), "completed", resp.Status)
	assert.Equal(s.T(), 10, resp.Total)
	assert.Equal(s.T(), 8, resp.ExactMatches)
	assert.Equal(s.T(), 1, resp.PartialMatches)
	assert.Equal(s.T(), 1, resp.Unmatched)
	assert.Equal(s.T(), "CycloneDX", resp.InputFormat)
}

func (s *CoverageReportDaoSuite) TestFetchNotFound() {
	_, err := s.dao().Fetch(context.Background(), seeds.RandomOrgId(), "00000000-0000-0000-0000-000000000099")
	require.Error(s.T(), err)
	var daoErr *ce.DaoError
	ok := errors.As(err, &daoErr)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *CoverageReportDaoSuite) TestFetchWrongOrg() {
	report := s.createReport("org-1", config.TaskStatusCompleted)

	_, err := s.dao().Fetch(context.Background(), "org-2", report.UUID)
	require.Error(s.T(), err)
	var daoErr *ce.DaoError
	ok := errors.As(err, &daoErr)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *CoverageReportDaoSuite) TestListPackages() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "spring-core", "6.1.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "lodash", "4.17.21", "NPM", models.CoverageMatchStatusPartial)
	s.createPackage(report.UUID, "unknown-pkg", "1.0.0", "Java", models.CoverageMatchStatusNone)

	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0}, api.ListCoverageReportPackagesRequest{})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), total)
	assert.Equal(s.T(), 3, len(resp.Data))

	byName := map[string]api.CoverageReportPackageResponse{}
	for _, p := range resp.Data {
		byName[p.Name] = p
	}
	assert.True(s.T(), byName["spring-core"].Covered)
	assert.Equal(s.T(), "6.1.0", byName["spring-core"].Version)
	assert.True(s.T(), byName["lodash"].Covered)
	assert.False(s.T(), byName["unknown-pkg"].Covered)
	assert.Equal(s.T(), models.CoverageMatchStatusExact, byName["spring-core"].MatchStatus)
	assert.Equal(s.T(), models.CoverageMatchStatusPartial, byName["lodash"].MatchStatus)
	assert.Equal(s.T(), models.CoverageMatchStatusNone, byName["unknown-pkg"].MatchStatus)
}

func (s *CoverageReportDaoSuite) TestListPackagesFilterByEcosystem() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "spring-core", "6.1.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "lodash", "4.17.21", "NPM", models.CoverageMatchStatusExact)

	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{Ecosystem: "Java"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Equal(s.T(), "Java", resp.Data[0].Ecosystem)

	resp, total, err = s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{Ecosystem: "Java,NPM"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
}

func (s *CoverageReportDaoSuite) TestListPackagesFilterBySearch() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "spring-core", "6.1.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "lodash", "4.17.21", "NPM", models.CoverageMatchStatusExact)

	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{Search: "spring"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Equal(s.T(), "spring-core", resp.Data[0].Name)
}

func (s *CoverageReportDaoSuite) TestListPackagesFilterByMatchStatus() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "spring-core", "6.1.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "lodash", "4.17.21", "NPM", models.CoverageMatchStatusPartial)

	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{MatchStatus: "exact"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Equal(s.T(), "spring-core", resp.Data[0].Name)

	resp, total, err = s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{MatchStatus: "partial"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Equal(s.T(), "lodash", resp.Data[0].Name)

	resp, total, err = s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{MatchStatus: "partial,exact"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
}

func (s *CoverageReportDaoSuite) TestListPackagesPagination() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "aaa", "1.0.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "bbb", "2.0.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "ccc", "3.0.0", "Java", models.CoverageMatchStatusExact)

	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 2, Offset: 0}, api.ListCoverageReportPackagesRequest{})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), total)
	assert.Equal(s.T(), 2, len(resp.Data))
	assert.Equal(s.T(), "aaa", resp.Data[0].Name)
	assert.Equal(s.T(), "bbb", resp.Data[1].Name)

	resp, total, err = s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 2, Offset: 2}, api.ListCoverageReportPackagesRequest{})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), total)
	assert.Equal(s.T(), 1, len(resp.Data))
	assert.Equal(s.T(), "ccc", resp.Data[0].Name)
}

func (s *CoverageReportDaoSuite) TestListPackagesReportNotFound() {
	_, _, err := s.dao().ListPackages(context.Background(), seeds.RandomOrgId(), "00000000-0000-0000-0000-000000000099",
		api.PaginationData{Limit: 100, Offset: 0}, api.ListCoverageReportPackagesRequest{})
	require.Error(s.T(), err)
	daoErr, ok := err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *CoverageReportDaoSuite) TestCreate() {
	uploadUUID := uuid.NewString()

	report, err := s.dao().Create(context.Background(),
		CreateCoverageReportParams{
			OrgID:     orgIDTest,
			AccountID: utils.Ptr("account-1"),
		},
		CreateCoverageUploadParams{
			UUID:       uploadUUID,
			StorageKey: "coverage-uploads/" + uploadUUID,
			Sha256:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			SizeBytes:  1024,
		},
	)
	require.NoError(s.T(), err)

	var readReport models.CoverageReport
	err = s.tx.Where("uuid = ?", report.UUID).First(&readReport).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusPending, readReport.Status)
	assert.Equal(s.T(), orgIDTest, readReport.OrgID)
	require.NotNil(s.T(), readReport.AccountID)
	assert.Equal(s.T(), "account-1", *readReport.AccountID)

	var readUpload models.CoverageUpload
	err = s.tx.Where("uuid = ?", uploadUUID).First(&readUpload).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "coverage-uploads/"+uploadUUID, readUpload.StorageKey)
	assert.Equal(s.T(), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", readUpload.Sha256)
	assert.Equal(s.T(), int64(1024), readUpload.SizeBytes)
}

func (s *CoverageReportDaoSuite) TestInternalOnlyFetchCoverageUpload() {
	modelUpload := s.createUpload()

	upload, err := s.dao().InternalOnlyFetchCoverageUpload(context.Background(), modelUpload.UUID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), modelUpload.UUID, upload.UUID)
	assert.Equal(s.T(), modelUpload.Sha256, upload.Sha256)
	assert.Equal(s.T(), modelUpload.SizeBytes, upload.SizeBytes)
	assert.Equal(s.T(), modelUpload.StorageKey, upload.StorageKey)
}

func (s *CoverageReportDaoSuite) TestInternalOnlyFetchCoverageUploadNotFound() {
	_, err := s.dao().InternalOnlyFetchCoverageUpload(context.Background(), uuid.NewString())
	require.Error(s.T(), err)
	var daoErr *ce.DaoError
	ok := errors.As(err, &daoErr)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *CoverageReportDaoSuite) TestSetAnalysisTaskUUID() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)
	taskUUID := uuid.NewString()

	err := s.dao().SetAnalysisTaskUUID(context.Background(), report.UUID, taskUUID)
	require.NoError(s.T(), err)

	var readReport models.CoverageReport
	err = s.tx.Where("uuid = ?", report.UUID).First(&readReport).Error
	require.NoError(s.T(), err)
	require.NotNil(s.T(), readReport.AnalysisTaskUUID)
	assert.Equal(s.T(), taskUUID, *readReport.AnalysisTaskUUID)
}

func (s *CoverageReportDaoSuite) TestSetAnalysisTaskUUIDNotFound() {
	err := s.dao().SetAnalysisTaskUUID(context.Background(), uuid.NewString(), uuid.NewString())
	require.Error(s.T(), err)
	var daoErr *ce.DaoError
	ok := errors.As(err, &daoErr)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *CoverageReportDaoSuite) TestUpdateCoverageReportStatus() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusPending)

	err := s.dao().UpdateCoverageReportStatus(context.Background(), report.UUID, config.TaskStatusRunning, nil)
	require.NoError(s.T(), err)
	var readReport models.CoverageReport
	err = s.tx.Where("uuid = ?", report.UUID).First(&readReport).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusRunning, readReport.Status)
	assert.Nil(s.T(), readReport.AnalysisTaskError)
	assert.Nil(s.T(), readReport.CompletedAt)

	errMsg := "analysis failed"
	err = s.dao().UpdateCoverageReportStatus(context.Background(), report.UUID, config.TaskStatusFailed, &errMsg)
	require.NoError(s.T(), err)

	err = s.tx.Where("uuid = ?", report.UUID).First(&readReport).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusFailed, readReport.Status)
	require.NotNil(s.T(), readReport.AnalysisTaskError)
	assert.Equal(s.T(), errMsg, *readReport.AnalysisTaskError)
	assert.NotNil(s.T(), readReport.CompletedAt)
}

func (s *CoverageReportDaoSuite) TestUpdateCoverageReportStatusNotFound() {
	err := s.dao().UpdateCoverageReportStatus(context.Background(), uuid.NewString(), config.TaskStatusRunning, nil)
	require.Error(s.T(), err)
	var daoErr *ce.DaoError
	ok := errors.As(err, &daoErr)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *CoverageReportDaoSuite) TestSaveCoverageAnalysis() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusRunning)
	snapshotAt := time.Now().UTC().Truncate(time.Microsecond)

	err := s.dao().SaveCoverageAnalysis(context.Background(), report.UUID, SaveCoverageAnalysisParams{
		InputFormat: "csv",
		Results: []matcher.MatchResult{
			{Package: matcher.Package{Ecosystem: "Java", Name: "spring-core", Version: "6.1.0", Namespace: "org.springframework"}, MatchStatus: matcher.MatchStatusExact},
			{Package: matcher.Package{Ecosystem: "Python", Name: "flask", Version: "2.0.0"}, MatchStatus: matcher.MatchStatusPartial},
			{Package: matcher.Package{Ecosystem: "Python", Name: "custom-lib", Version: "0.1.0"}, MatchStatus: matcher.MatchStatusNone},
		},
		Summary: matcher.MatchSummary{
			Total:          3,
			ExactMatches:   1,
			PartialMatches: 1,
			Unmatched:      1,
			EcosystemCoverageSummary: []matcher.EcosystemSummary{
				{Ecosystem: "Java", Total: 1, ExactMatches: 1},
				{Ecosystem: "Python", Total: 2, PartialMatches: 1, Unmatched: 1},
			},
			CatalogSnapshotAt: snapshotAt,
		},
	})
	require.NoError(s.T(), err)

	var readReport models.CoverageReport
	err = s.tx.Where("uuid = ?", report.UUID).First(&readReport).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusCompleted, readReport.Status)
	require.NotNil(s.T(), readReport.InputFormat)
	assert.Equal(s.T(), "csv", *readReport.InputFormat)
	require.NotNil(s.T(), readReport.Total)
	assert.Equal(s.T(), 3, *readReport.Total)
	require.NotNil(s.T(), readReport.ExactMatches)
	assert.Equal(s.T(), 1, *readReport.ExactMatches)
	require.NotNil(s.T(), readReport.PartialMatches)
	assert.Equal(s.T(), 1, *readReport.PartialMatches)
	require.NotNil(s.T(), readReport.Unmatched)
	assert.Equal(s.T(), 1, *readReport.Unmatched)
	assert.NotNil(s.T(), readReport.CompletedAt)
	require.NotNil(s.T(), readReport.CatalogSnapshotAt)
	assert.True(s.T(), snapshotAt.Equal(*readReport.CatalogSnapshotAt))
	require.NotNil(s.T(), readReport.EcosystemCoverageSummary)
	assert.Len(s.T(), *readReport.EcosystemCoverageSummary, 2)

	var packages []models.CoverageReportPackage
	err = s.tx.Where("coverage_report_uuid = ?", report.UUID).Order("name ASC").Find(&packages).Error
	require.NoError(s.T(), err)
	require.Len(s.T(), packages, 3)
	assert.Equal(s.T(), "custom-lib", packages[0].Name)
	assert.Nil(s.T(), packages[0].Namespace)
	assert.Equal(s.T(), models.CoverageMatchStatusNone, packages[0].MatchStatus)
	assert.Equal(s.T(), "flask", packages[1].Name)
	assert.Equal(s.T(), models.CoverageMatchStatusPartial, packages[1].MatchStatus)
	assert.Equal(s.T(), "spring-core", packages[2].Name)
	require.NotNil(s.T(), packages[2].Namespace)
	assert.Equal(s.T(), "org.springframework", *packages[2].Namespace)
	assert.Equal(s.T(), models.CoverageMatchStatusExact, packages[2].MatchStatus)

	var signals []models.CoverageDemandSignal
	err = s.tx.Order("name ASC").Find(&signals).Error
	require.NoError(s.T(), err)
	require.Len(s.T(), signals, 2)
	assert.Equal(s.T(), "custom-lib", signals[0].Name)
	assert.Equal(s.T(), models.CoverageDemandMatchStatusNone, signals[0].MatchStatus)
	assert.Equal(s.T(), models.CoverageDemandSourceProspectDriven, signals[0].Source)
	assert.Equal(s.T(), "flask", signals[1].Name)
	assert.Equal(s.T(), models.CoverageDemandMatchStatusPartial, signals[1].MatchStatus)
}

func (s *CoverageReportDaoSuite) TestSaveCoverageAnalysisNotFound() {
	err := s.dao().SaveCoverageAnalysis(context.Background(), uuid.NewString(), SaveCoverageAnalysisParams{
		InputFormat: "csv",
		Summary:     matcher.MatchSummary{CatalogSnapshotAt: time.Now().UTC()},
	})
	require.Error(s.T(), err)
	var daoErr *ce.DaoError
	ok := errors.As(err, &daoErr)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}
