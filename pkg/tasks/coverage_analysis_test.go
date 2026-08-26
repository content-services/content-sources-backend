package tasks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type CoverageAnalysisSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	s3Mock          *s3_client.MockS3Client
	mockPulp        *pulp_client.MockPulpClient
	mockTang        *tangy.MockTangy
}

func TestCoverageAnalysisSuite(t *testing.T) {
	suite.Run(t, new(CoverageAnalysisSuite))
}

func (s *CoverageAnalysisSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.s3Mock = s3_client.NewMockS3Client(s.T())
	s.mockPulp = pulp_client.NewMockPulpClient(s.T())
	s.mockTang = tangy.NewMockTangy(s.T())
}

func (s *CoverageAnalysisSuite) newTask(ctx context.Context, payload *payloads.CoverageAnalysisPayload) *CoverageAnalysis {
	return &CoverageAnalysis{
		ctx:        ctx,
		payload:    payload,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		s3Client:   s.s3Mock,
		pulpClient: s.mockPulp,
		tang:       s.mockTang,
	}
}

func (s *CoverageAnalysisSuite) mockFetchManifest(ctx context.Context, reportUUID, filename string, manifest []byte) payloads.CoverageAnalysisPayload {
	hash := sha256.Sum256(manifest)
	upload := models.CoverageUpload{
		UUID:       uuid.NewString(),
		StorageKey: "coverage-uploads/" + uuid.NewString(),
		Sha256:     hex.EncodeToString(hash[:]),
		SizeBytes:  int64(len(manifest)),
	}
	s.mockDaoRegistry.CoverageReport.On("InternalOnlyFetchCoverageUpload", ctx, upload.UUID).Return(upload, nil).Once()
	s.s3Mock.On("Get", ctx, upload.StorageKey).Return(io.NopCloser(bytes.NewReader(manifest)), nil).Once()
	return payloads.CoverageAnalysisPayload{
		CoverageReportUUID: reportUUID,
		CoverageUploadUUID: upload.UUID,
		Filename:           filename,
	}
}

func (s *CoverageAnalysisSuite) mockCatalogRepos(ctx context.Context) {
	domainName := "test-domain"
	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", ctx, config.LightwellOrg).Return(domainName, nil)
	s.mockPulp.On("WithDomain", domainName).Return(s.mockPulp)
	s.mockDaoRegistry.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", ctx, config.LightwellOrg).Return([]api.RepositoryResponse{
		{
			UUID:                  "repo-1",
			Name:                  "test-repo-1",
			PublishedDistBasePath: "java/validated",
			ContentType:           config.ContentTypeMaven,
		},
		{
			UUID:                  "repo-2",
			Name:                  "test-repo-2",
			PublishedDistBasePath: "python/validated",
			ContentType:           config.ContentTypePython,
		},
	}, nil)
}

func (s *CoverageAnalysisSuite) mockJavaCatalog(ctx context.Context, groupID, artifactID, version string) {
	javaHref := "test-repo-href-1"
	s.mockPulp.On("ResolveRepositoryFromBasePath", ctx, "java/validated").Return(utils.Ptr(javaHref), nil)
	s.mockTang.On("MavenPackageList", ctx, javaHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: tangy.DefaultLimit}).
		Return(tangy.MavenPackageListResponse{
			Results: []tangy.MavenPackageListItem{
				{GroupID: groupID, ArtifactID: artifactID, Versions: []string{version}},
			},
			Total:  1,
			Limit:  tangy.DefaultLimit,
			Offset: 0,
		}, nil)
}

func (s *CoverageAnalysisSuite) mockPythonCatalog(ctx context.Context, name, version string) {
	pythonHref := "test-repo-href-2"
	s.mockPulp.On("ResolveRepositoryFromBasePath", ctx, "python/validated").Return(utils.Ptr(pythonHref), nil)
	s.mockTang.On("PythonPackageList", ctx, pythonHref, tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: tangy.DefaultLimit}).
		Return(tangy.PythonPackageListResponse{
			Results: []tangy.PythonPackageListItem{
				{Name: name, NameNormalized: name, Versions: []string{version}},
			},
			Total:  1,
			Limit:  tangy.DefaultLimit,
			Offset: 0,
		}, nil)
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysis() {
	ctx := context.Background()
	reportUUID := uuid.NewString()
	manifest := []byte("packageurl\npkg:maven/commons-io/commons-io@2.11.0\npkg:pypi/flask@3.0.3\n")
	payload := s.mockFetchManifest(ctx, reportUUID, "manifest.csv", manifest)
	s.mockDaoRegistry.CoverageReport.On("UpdateCoverageReportStatus", ctx, reportUUID, config.TaskStatusRunning, (*string)(nil)).Return(nil).Once()
	s.mockCatalogRepos(ctx)
	s.mockJavaCatalog(ctx, "commons-io", "commons-io", "2.11.0")
	s.mockPythonCatalog(ctx, "flask", "3.0.3")

	s.mockDaoRegistry.CoverageReport.On("SaveCoverageAnalysis", ctx, reportUUID, mock.Anything).Return(nil).Once()

	err := s.newTask(ctx, &payload).Run()
	require.NoError(s.T(), err)
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysisVerifyManifest() {
	ctx := context.Background()
	manifest := []byte("packageurl\npkg:maven/commons-io/commons-io@2.11.0\n")
	upload := models.CoverageUpload{
		UUID:       uuid.NewString(),
		StorageKey: "coverage-uploads/" + uuid.NewString(),
		Sha256:     "wrongsha",
		SizeBytes:  int64(len(manifest)),
	}
	s.mockDaoRegistry.CoverageReport.On("InternalOnlyFetchCoverageUpload", ctx, upload.UUID).
		Return(upload, nil).Once()
	s.s3Mock.On("Get", ctx, upload.StorageKey).
		Return(io.NopCloser(bytes.NewReader(manifest)), nil).Once()

	payload := payloads.CoverageAnalysisPayload{
		CoverageReportUUID: uuid.NewString(),
		CoverageUploadUUID: upload.UUID,
		Filename:           "manifest.csv",
	}
	err := s.newTask(ctx, &payload).Run()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "sha256 mismatch")
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysisUploadNotFound() {
	ctx := context.Background()
	uploadUUID := uuid.NewString()
	payload := payloads.CoverageAnalysisPayload{
		CoverageReportUUID: uuid.NewString(),
		CoverageUploadUUID: uploadUUID,
	}

	s.mockDaoRegistry.CoverageReport.On("InternalOnlyFetchCoverageUpload", ctx, uploadUUID).
		Return(models.CoverageUpload{}, gorm.ErrRecordNotFound).Once()

	err := s.newTask(ctx, &payload).Run()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "fetch upload")
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysisS3GetFails() {
	ctx := context.Background()
	upload := models.CoverageUpload{
		UUID:       uuid.NewString(),
		StorageKey: "coverage-uploads/" + uuid.NewString(),
	}

	s.mockDaoRegistry.CoverageReport.On("InternalOnlyFetchCoverageUpload", ctx, upload.UUID).
		Return(upload, nil).Once()
	s.s3Mock.On("Get", ctx, upload.StorageKey).Return(nil, fmt.Errorf("s3 unavailable")).Once()

	payload := payloads.CoverageAnalysisPayload{
		CoverageReportUUID: uuid.NewString(),
		CoverageUploadUUID: upload.UUID,
	}
	err := s.newTask(ctx, &payload).Run()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "download manifest")
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysisFilenameUnsupported() {
	ctx := context.Background()
	reportUUID := uuid.NewString()
	payload := s.mockFetchManifest(ctx, reportUUID, "manifest.json", []byte(`{"components":[]}`))

	err := s.newTask(ctx, &payload).Run()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "parse manifest")
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysisManifestEmpty() {
	ctx := context.Background()
	reportUUID := uuid.NewString()
	payload := s.mockFetchManifest(ctx, reportUUID, "manifest.csv", []byte("packageurl\n"))

	err := s.newTask(ctx, &payload).Run()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "no packages found in manifest")
}

func (s *CoverageAnalysisSuite) TestCoverageAnalysisCatalogLoadFails() {
	ctx := context.Background()
	reportUUID := uuid.NewString()
	manifest := []byte("packageurl\npkg:maven/commons-io/commons-io@2.11.0\n")
	payload := s.mockFetchManifest(ctx, reportUUID, "manifest.csv", manifest)
	s.mockDaoRegistry.CoverageReport.On("UpdateCoverageReportStatus", ctx, reportUUID, config.TaskStatusRunning, (*string)(nil)).Return(nil).Once()

	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", ctx, config.LightwellOrg).
		Return("", errors.New("domain unavailable"))

	err := s.newTask(ctx, &payload).Run()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "load catalog")
}
