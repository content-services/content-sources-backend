package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CatalogSuite struct {
	suite.Suite
	mockDao  *dao.MockDaoRegistry
	mockPulp *pulp_client.MockPulpClient
	mockTang *tangy.MockTangy
	ctx      context.Context
}

func TestCatalogSuite(t *testing.T) {
	suite.Run(t, new(CatalogSuite))
}

func (s *CatalogSuite) SetupTest() {
	s.mockDao = dao.GetMockDaoRegistry(s.T())
	s.mockPulp = pulp_client.NewMockPulpClient(s.T())
	s.mockTang = tangy.NewMockTangy(s.T())
	s.ctx = context.Background()
}

func (s *CatalogSuite) TestLoadCatalog() {
	domainName := "test-domain"
	javaHref := "test-repo-href-1"
	pythonHref := "test-repo-href-2"

	repos := []api.RepositoryResponse{
		{
			UUID:                  "repo-1",
			Name:                  "test-repo-1",
			PublishedDistBasePath: "java/validated",
			ContentType:           config.ContentTypeMaven,
		},
		{
			UUID:                  "repo-skip",
			Name:                  "test-repo-skip",
			PublishedDistBasePath: "java/remediated",
			ContentType:           config.ContentTypeMaven,
		},
		{
			UUID:                  "repo-2",
			Name:                  "test-repo-2",
			PublishedDistBasePath: "python/validated",
			ContentType:           config.ContentTypePython,
		},
	}

	s.mockDao.Domain.On("FetchOrCreateDomain", s.ctx, config.LightwellOrg).Return(domainName, nil)
	s.mockPulp.On("WithDomain", domainName).Return(s.mockPulp)
	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)

	s.mockPulp.On("ResolveRepositoryFromBasePath", s.ctx, "java/validated").Return(utils.Ptr(javaHref), nil)
	s.mockTang.On("MavenPackageList", s.ctx, javaHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: tangy.DefaultLimit}).
		Return(tangy.MavenPackageListResponse{
			Results: []tangy.MavenPackageListItem{
				{
					GroupID:    "org.springframework",
					ArtifactID: "spring-core",
					Versions:   []string{"6.1.0", "6.0.0"},
				},
			},
			Total:  1,
			Limit:  tangy.DefaultLimit,
			Offset: 0,
		}, nil)

	s.mockPulp.On("ResolveRepositoryFromBasePath", s.ctx, "python/validated").Return(utils.Ptr(pythonHref), nil)
	s.mockTang.On("PythonPackageList", s.ctx, pythonHref, tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: tangy.DefaultLimit}).
		Return(tangy.PythonPackageListResponse{
			Results: []tangy.PythonPackageListItem{
				{
					Name:           "Flask",
					NameNormalized: "flask",
					Versions:       []string{"3.0.3", "2.3.0"},
				},
			},
			Total:  1,
			Limit:  tangy.DefaultLimit,
			Offset: 0,
		}, nil)

	catalog, snapshotAt, err := LoadCatalog(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulp, s.mockTang)
	require.NoError(s.T(), err)
	assert.False(s.T(), snapshotAt.IsZero())
	assert.ElementsMatch(s.T(), []matcher.Package{
		{Ecosystem: matcher.EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "6.1.0"},
		{Ecosystem: matcher.EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "6.0.0"},
		{Ecosystem: matcher.EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: matcher.EcosystemPython, Name: "flask", Version: "2.3.0"},
	}, catalog)
}

func (s *CatalogSuite) TestLoadCatalogPagination() {
	domainName := "test-domain"
	javaHref := "test-repo-href-1"

	repos := []api.RepositoryResponse{
		{
			UUID:                  "repo-1",
			Name:                  "test-repo-1",
			PublishedDistBasePath: "java/validated",
			ContentType:           config.ContentTypeMaven,
		},
	}

	s.mockDao.Domain.On("FetchOrCreateDomain", s.ctx, config.LightwellOrg).Return(domainName, nil)
	s.mockPulp.On("WithDomain", domainName).Return(s.mockPulp)
	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)
	s.mockPulp.On("ResolveRepositoryFromBasePath", s.ctx, "java/validated").Return(utils.Ptr(javaHref), nil)

	s.mockTang.On("MavenPackageList", s.ctx, javaHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: tangy.DefaultLimit}).
		Return(tangy.MavenPackageListResponse{
			Results: []tangy.MavenPackageListItem{
				{GroupID: "org.springframework", ArtifactID: "spring-core", Versions: []string{"6.1.0"}},
			},
			Total:  tangy.DefaultLimit + 1,
			Limit:  tangy.DefaultLimit,
			Offset: 0,
		}, nil)
	s.mockTang.On("MavenPackageList", s.ctx, javaHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: tangy.DefaultLimit, Limit: tangy.DefaultLimit}).
		Return(tangy.MavenPackageListResponse{
			Results: []tangy.MavenPackageListItem{
				{GroupID: "com.google.guava", ArtifactID: "guava", Versions: []string{"32.0"}},
			},
			Total:  tangy.DefaultLimit + 1,
			Limit:  tangy.DefaultLimit,
			Offset: tangy.DefaultLimit,
		}, nil)

	catalog, _, err := LoadCatalog(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulp, s.mockTang)
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []matcher.Package{
		{Ecosystem: matcher.EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "6.1.0"},
		{Ecosystem: matcher.EcosystemJava, Namespace: "com.google.guava", Name: "guava", Version: "32.0"},
	}, catalog)
}

func (s *CatalogSuite) TestLoadCatalogNoValidatedRepos() {
	domainName := "test-domain"
	repos := []api.RepositoryResponse{
		{
			UUID:                  "repo-skip",
			Name:                  "test-repo-skip",
			PublishedDistBasePath: "java/remediated",
			ContentType:           config.ContentTypeMaven,
		},
	}

	s.mockDao.Domain.On("FetchOrCreateDomain", s.ctx, config.LightwellOrg).Return(domainName, nil)
	s.mockPulp.On("WithDomain", domainName).Return(s.mockPulp)
	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)

	_, _, err := LoadCatalog(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulp, s.mockTang)
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "no validated")
}

func (s *CatalogSuite) TestLoadCatalogTangError() {
	domainName := "test-domain"
	javaHref := "test-repo-href-1"
	repos := []api.RepositoryResponse{
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
	}

	s.mockDao.Domain.On("FetchOrCreateDomain", s.ctx, config.LightwellOrg).Return(domainName, nil)
	s.mockPulp.On("WithDomain", domainName).Return(s.mockPulp)
	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)
	s.mockPulp.On("ResolveRepositoryFromBasePath", s.ctx, "java/validated").Return(utils.Ptr(javaHref), nil)
	s.mockTang.On("MavenPackageList", s.ctx, javaHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: tangy.DefaultLimit}).
		Return(tangy.MavenPackageListResponse{}, errors.New("tang unavailable"))

	_, _, err := LoadCatalog(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulp, s.mockTang)
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "tang unavailable")
}

func (s *CatalogSuite) TestLoadCatalogPulpError() {
	domainName := "test-domain"
	repos := []api.RepositoryResponse{
		{
			UUID:                  "repo-1",
			Name:                  "test-repo-1",
			PublishedDistBasePath: "java/validated",
			ContentType:           config.ContentTypeMaven,
		},
	}

	s.mockDao.Domain.On("FetchOrCreateDomain", s.ctx, config.LightwellOrg).Return(domainName, nil)
	s.mockPulp.On("WithDomain", domainName).Return(s.mockPulp)
	s.mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigForOrg", s.ctx, config.LightwellOrg).Return(repos, nil)
	s.mockPulp.On("ResolveRepositoryFromBasePath", s.ctx, "java/validated").Return((*string)(nil), errors.New("pulp unavailable"))

	_, _, err := LoadCatalog(s.ctx, s.mockDao.ToDaoRegistry(), s.mockPulp, s.mockTang)
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "pulp unavailable")
}
