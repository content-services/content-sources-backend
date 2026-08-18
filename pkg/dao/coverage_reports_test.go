package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
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
}

func (s *CoverageReportDaoSuite) TestListPackagesFilterByCoveredTrue() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "spring-core", "6.1.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "lodash", "4.17.21", "NPM", models.CoverageMatchStatusPartial)
	s.createPackage(report.UUID, "unknown-pkg", "1.0.0", "Java", models.CoverageMatchStatusNone)

	covered := true
	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{Covered: &covered})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	for _, p := range resp.Data {
		assert.True(s.T(), p.Covered)
	}
}

func (s *CoverageReportDaoSuite) TestListPackagesFilterByCoveredFalse() {
	orgID := seeds.RandomOrgId()
	report := s.createReport(orgID, config.TaskStatusCompleted)

	s.createPackage(report.UUID, "spring-core", "6.1.0", "Java", models.CoverageMatchStatusExact)
	s.createPackage(report.UUID, "unknown-pkg", "1.0.0", "Java", models.CoverageMatchStatusNone)

	covered := false
	resp, total, err := s.dao().ListPackages(context.Background(), orgID, report.UUID,
		api.PaginationData{Limit: 100, Offset: 0},
		api.ListCoverageReportPackagesRequest{Covered: &covered})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Equal(s.T(), "unknown-pkg", resp.Data[0].Name)
	assert.False(s.T(), resp.Data[0].Covered)
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
	dao := coverageReportDaoImpl{db: s.tx}

	uploadUUID := uuid.NewString()
	report, err := dao.Create(context.Background(),
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
	assert.Equal(s.T(), "account-1", *readReport.AccountID)

	var readUpload models.CoverageUpload
	err = s.tx.Where("uuid = ?", uploadUUID).First(&readUpload).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "coverage-uploads/"+uploadUUID, readUpload.StorageKey)
	assert.Equal(s.T(), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", readUpload.Sha256)
	assert.Equal(s.T(), int64(1024), readUpload.SizeBytes)
}
