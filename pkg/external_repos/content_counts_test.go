package external_repos

import (
	"context"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type ContentCountsSuite struct {
	suite.Suite
	mockDao        *dao.MockDaoRegistry
	mockPulpClient *pulp_client.MockPulpClient
	mockTangy      *tangy.MockTangy
	mockCache      *cache.MockCache
	ctx            context.Context
}

func TestContentCountsSuite(t *testing.T) {
	suite.Run(t, new(ContentCountsSuite))
}

func (s *ContentCountsSuite) SetupTest() {
	s.mockDao = dao.GetMockDaoRegistry(s.T())
	s.mockPulpClient = pulp_client.NewMockPulpClient(s.T())
	s.mockTangy = tangy.NewMockTangy(s.T())
	s.mockCache = cache.NewMockCache(s.T())
	s.ctx = context.Background()
}

func (s *ContentCountsSuite) TestUpdateContentCountsWithCache_Success() {
	t := s.T()
	domainName := "test-domain"
	repoUUID1 := uuid.NewString()
	repoUUID2 := uuid.NewString()
	repoHref1 := "test-repo-href-1"
	repoHref2 := "test-repo-href-2"

	repos := []api.RepositoryResponse{
		{
			UUID:                  repoUUID1,
			Name:                  "test-repo-1",
			PublishedDistBasePath: "/base/path/1",
			RepositoryUUID:        repoUUID1,
			ContentType:           config.ContentTypeMaven,
			PackageCount:          0,
			BuildCount:            0,
		},
		{
			UUID:                  repoUUID2,
			Name:                  "test-repo-2",
			PublishedDistBasePath: "/base/path/2",
			RepositoryUUID:        repoUUID2,
			ContentType:           config.ContentTypePython,
			PackageCount:          5,
			BuildCount:            5,
		},
	}

	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)

	// First repo - cache miss, fetch from pulp
	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID1).Return(nil, cache.ErrNotFound)
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path/1").Return(&repoHref1, nil)
	s.mockTangy.On("MavenRepositoryMetrics", s.ctx, repoHref1).
		Return(tangy.MavenRepositoryMetrics{PackageCount: 10, BuildCount: 8}, nil)
	s.mockCache.On("SetContentCounts", s.ctx, domainName, repoUUID1, cache.RepoContentCount{
		Packages: 10,
		Builds:   8,
	}).Return(nil)
	s.mockDao.Repository.On("InternalOnly_UpdateCounts", s.ctx, repoUUID1, 10, 8).Return(nil)

	// Second repo - cache hit
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path/2").Return(&repoHref2, nil)
	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID2).Return(&cache.RepoContentCount{
		Packages: 5,
		Builds:   5,
	}, nil)

	err := UpdateContentCountsWithCache(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulpClient, s.mockTangy, s.mockCache, domainName)
	assert.NoError(t, err)
}

func (s *ContentCountsSuite) TestUpdateContentCountsWithCache_FetchRepoConfigError() {
	t := s.T()
	domainName := "test-domain"
	expectedErr := errors.New("database error")

	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(nil, expectedErr)

	err := UpdateContentCountsWithCache(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulpClient, s.mockTangy, s.mockCache, domainName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch repoConfig")
}

func (s *ContentCountsSuite) TestGetContentCountsWithCache_CacheHit() {
	t := s.T()
	domainName := "test-domain"
	repoUUID := uuid.NewString()

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		Name:                  "test-repo",
		PublishedDistBasePath: "/base/path",
		ContentType:           config.ContentTypeMaven,
	}

	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID).Return(&cache.RepoContentCount{
		Packages: 100,
		Builds:   50,
	}, nil)

	pkgCount, buildCount, updated, err := GetContentCountsWithCache(s.ctx, s.mockPulpClient, s.mockTangy, s.mockCache, domainName, repo)
	assert.NoError(t, err)
	assert.Equal(t, 100, pkgCount)
	assert.Equal(t, 50, buildCount)
	assert.False(t, updated)
}

func (s *ContentCountsSuite) TestGetContentCountsWithCache_CacheMiss() {
	t := s.T()
	domainName := "test-domain"
	repoUUID := uuid.NewString()
	repoHref := "test-repo-href"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		Name:                  "test-repo",
		PublishedDistBasePath: "/base/path",
		ContentType:           config.ContentTypeMaven,
	}

	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID).Return(nil, cache.ErrNotFound)
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path").Return(&repoHref, nil)
	s.mockTangy.On("MavenRepositoryMetrics", s.ctx, repoHref).
		Return(tangy.MavenRepositoryMetrics{PackageCount: 25, BuildCount: 15}, nil)
	s.mockCache.On("SetContentCounts", s.ctx, domainName, repoUUID, cache.RepoContentCount{
		Packages: 25,
		Builds:   15,
	}).Return(nil)

	pkgCount, buildCount, updated, err := GetContentCountsWithCache(s.ctx, s.mockPulpClient, s.mockTangy, s.mockCache, domainName, repo)
	assert.NoError(t, err)
	assert.Equal(t, 25, pkgCount)
	assert.Equal(t, 15, buildCount)
	assert.True(t, updated)
}

func (s *ContentCountsSuite) TestGetContentCountsWithCache_ResolveRepoError() {
	t := s.T()
	domainName := "test-domain"
	repoUUID := uuid.NewString()

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		Name:                  "test-repo",
		PublishedDistBasePath: "/base/path",
		ContentType:           config.ContentTypeMaven,
	}

	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID).Return(nil, cache.ErrNotFound)
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path").Return(nil, errors.New("resolve error"))

	pkgCount, buildCount, updated, err := GetContentCountsWithCache(s.ctx, s.mockPulpClient, s.mockTangy, s.mockCache, domainName, repo)
	assert.Error(t, err)
	assert.Equal(t, 0, pkgCount)
	assert.Equal(t, 0, buildCount)
	assert.False(t, updated)
}

func (s *ContentCountsSuite) TestGetContentCountsWithCache_NilRepoHref() {
	t := s.T()
	domainName := "test-domain"
	repoUUID := uuid.NewString()

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		Name:                  "test-repo",
		PublishedDistBasePath: "/base/path",
		ContentType:           config.ContentTypeMaven,
	}

	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID).Return(nil, cache.ErrNotFound)
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path").Return(nil, nil)

	pkgCount, buildCount, updated, err := GetContentCountsWithCache(s.ctx, s.mockPulpClient, s.mockTangy, s.mockCache, domainName, repo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve repo")
	assert.Equal(t, 0, pkgCount)
	assert.Equal(t, 0, buildCount)
	assert.False(t, updated)
}

func (s *ContentCountsSuite) TestContentCountsForType_Maven() {
	t := s.T()
	repoHref := "test-repo-href"

	s.mockTangy.On("MavenRepositoryMetrics", s.ctx, repoHref).
		Return(tangy.MavenRepositoryMetrics{PackageCount: 30, BuildCount: 20}, nil)

	pkgCount, buildCount, err := ContentCountsForType(s.ctx, s.mockTangy, repoHref, config.ContentTypeMaven)
	assert.NoError(t, err)
	assert.Equal(t, 30, pkgCount)
	assert.Equal(t, 20, buildCount)
}

func (s *ContentCountsSuite) TestContentCountsForType_Python() {
	t := s.T()
	repoHref := "test-repo-href"

	s.mockTangy.On("PythonRepositoryMetrics", s.ctx, repoHref).
		Return(tangy.PythonRepositoryMetrics{PackageCount: 42, BuildCount: 2}, nil)

	pkgCount, buildCount, err := ContentCountsForType(s.ctx, s.mockTangy, repoHref, config.ContentTypePython)
	assert.NoError(t, err)
	assert.Equal(t, 42, pkgCount)
	assert.Equal(t, 2, buildCount)
}

func (s *ContentCountsSuite) TestContentCountsForType_UnknownType() {
	t := s.T()
	repoHref := "test-repo-href"

	pkgCount, buildCount, err := ContentCountsForType(s.ctx, s.mockTangy, repoHref, "unknown-type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown content type")
	assert.Equal(t, 0, pkgCount)
	assert.Equal(t, 0, buildCount)
}

func (s *ContentCountsSuite) TestContentCountsForType_MavenRepositoryMetricsError() {
	t := s.T()
	repoHref := "test-repo-href"

	s.mockTangy.On("MavenRepositoryMetrics", s.ctx, repoHref).
		Return(tangy.MavenRepositoryMetrics{}, errors.New("repository metrics error"))

	pkgCount, buildCount, err := ContentCountsForType(s.ctx, s.mockTangy, repoHref, config.ContentTypeMaven)
	assert.Error(t, err)
	assert.Equal(t, 0, pkgCount)
	assert.Equal(t, 0, buildCount)
}

func (s *ContentCountsSuite) TestContentCountsForType_PythonRepositoryMetricsError() {
	t := s.T()
	repoHref := "test-repo-href"

	s.mockTangy.On("PythonRepositoryMetrics", s.ctx, repoHref).
		Return(tangy.PythonRepositoryMetrics{}, errors.New("repository metrics error"))

	pkgCount, buildCount, err := ContentCountsForType(s.ctx, s.mockTangy, repoHref, config.ContentTypePython)
	assert.Error(t, err)
	assert.Equal(t, 0, pkgCount)
	assert.Equal(t, 0, buildCount)
}

func (s *ContentCountsSuite) TestUpdateContentCountsWithCache_SkipUpdateWhenCountsMatch() {
	t := s.T()
	domainName := "test-domain"
	repoUUID := uuid.NewString()
	repoHref := "test-repo-href"

	repos := []api.RepositoryResponse{
		{
			UUID:                  repoUUID,
			Name:                  "test-repo",
			PublishedDistBasePath: "/base/path",
			RepositoryUUID:        repoUUID,
			ContentType:           config.ContentTypePython,
			PackageCount:          42,
			BuildCount:            42,
		},
	}

	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path").Return(&repoHref, nil)
	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID).Return(&cache.RepoContentCount{
		Packages: 42,
		Builds:   42,
	}, nil)

	err := UpdateContentCountsWithCache(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulpClient, s.mockTangy, s.mockCache, domainName)
	assert.NoError(t, err)
}

func (s *ContentCountsSuite) TestUpdateContentCountsWithCache_ContinuesOnError() {
	t := s.T()
	domainName := "test-domain"
	repoUUID1 := uuid.NewString()
	repoUUID2 := uuid.NewString()
	repoHref2 := "test-repo-href-2"

	repos := []api.RepositoryResponse{
		{
			UUID:                  repoUUID1,
			Name:                  "test-repo-1",
			PublishedDistBasePath: "/base/path/1",
			RepositoryUUID:        repoUUID1,
			ContentType:           config.ContentTypeMaven,
			PackageCount:          0,
			BuildCount:            0,
		},
		{
			UUID:                  repoUUID2,
			Name:                  "test-repo-2",
			PublishedDistBasePath: "/base/path/2",
			RepositoryUUID:        repoUUID2,
			ContentType:           config.ContentTypePython,
			PackageCount:          0,
			BuildCount:            0,
		},
	}

	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)

	// First repo - resolve fails (continues without calling GetContentCounts)
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path/1").Return(nil, errors.New("resolve error"))

	// Second repo - succeeds
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path/2").Return(&repoHref2, nil)
	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID2).Return(nil, cache.ErrNotFound)
	s.mockTangy.On("PythonRepositoryMetrics", s.ctx, repoHref2).
		Return(tangy.PythonRepositoryMetrics{PackageCount: 10, BuildCount: 2}, nil)

	s.mockCache.On("SetContentCounts", s.ctx, domainName, repoUUID2, cache.RepoContentCount{
		Packages: 10,
		Builds:   2,
	}).Return(nil)
	s.mockDao.Repository.On("InternalOnly_UpdateCounts", s.ctx, repoUUID2, 10, 2).Return(nil)

	err := UpdateContentCountsWithCache(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulpClient, s.mockTangy, s.mockCache, domainName)
	assert.NoError(t, err)
}

func (s *ContentCountsSuite) TestGetContentCountsWithCache_CacheReadError() {
	t := s.T()
	domainName := "test-domain"
	repoUUID := uuid.NewString()
	repoHref := "test-repo-href"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		Name:                  "test-repo",
		PublishedDistBasePath: "/base/path",
		ContentType:           config.ContentTypePython,
	}

	s.mockCache.On("GetContentCounts", s.ctx, domainName, repoUUID).Return(nil, errors.New("cache read error"))
	s.mockPulpClient.On("ResolveRepositoryFromBasePath", s.ctx, "/base/path").Return(&repoHref, nil)
	s.mockTangy.On("PythonRepositoryMetrics", s.ctx, repoHref).
		Return(tangy.PythonRepositoryMetrics{PackageCount: 5, BuildCount: 2}, nil)
	s.mockCache.On("SetContentCounts", s.ctx, domainName, repoUUID, mock.Anything).Return(nil)

	pkgCount, buildCount, updated, err := GetContentCountsWithCache(s.ctx, s.mockPulpClient, s.mockTangy, s.mockCache, domainName, repo)
	assert.NoError(t, err)
	assert.Equal(t, 5, pkgCount)
	assert.Equal(t, 2, buildCount)
	assert.True(t, updated)
}
