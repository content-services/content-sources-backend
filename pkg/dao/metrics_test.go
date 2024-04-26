package dao

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	*DaoSuite

	dao MetricsDao

	initialRepoCount                                  int
	initialRepositoryConfigsCount                     int
	initialPublicRepositoriesIntrospectionCount       IntrospectionCount
	initialPublicRepositoriesFailedIntrospectionCount int
	initialCustomRepositoriesIntrospectionCount       IntrospectionCount
}

func (s *MetricsSuite) SetupTest() {
	s.DaoSuite.SetupTest()
	s.dao = GetMetricsDao(s.tx)

	s.initialRepoCount = s.dao.RepositoriesCount(context.Background())
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialRepositoryConfigsCount = s.dao.RepositoryConfigsCount(context.Background())
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialPublicRepositoriesIntrospectionCount = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, true)
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialCustomRepositoriesIntrospectionCount = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, false)
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialPublicRepositoriesFailedIntrospectionCount = s.dao.PublicRepositoriesFailedIntrospectionCount(context.Background())
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
}

func TestMetricsSuite(t *testing.T) {
	m := DaoSuite{}
	r := MetricsSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *MetricsSuite) TestGetMetricsDao() {
	t := s.T()

	var dao MetricsDao

	dao = GetMetricsDao(nil)
	assert.Nil(t, dao)

	dao = GetMetricsDao(s.tx)
	assert.NotNil(t, dao)
}

func (s *MetricsSuite) TestOrganizationCount() {
	t := s.T()
	dao := s.dao
	var result int64

	err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{})
	assert.Nil(t, err)

	// The initial state should be 0
	result = dao.OrganizationTotal(context.Background())
	assert.True(t, result > 0)
}

func (s *MetricsSuite) TestRepositoriesCount() {
	t := s.T()
	dao := s.dao
	var result int

	// The initial state should be 0
	result = dao.RepositoriesCount(context.Background())
	assert.Equal(t, 0, result-s.initialRepoCount)

	// The counter is increased by 1
	s.tx.Create(&models.Repository{
		URL:                          "https://",
		Public:                       true,
		LastIntrospectionStatus:      config.StatusInvalid,
		LastIntrospectionTime:        nil,
		LastIntrospectionSuccessTime: nil,
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionError:       nil,
		PackageCount:                 0,
	})

	result = dao.RepositoriesCount(context.Background())
	assert.Equal(t, 1, result-s.initialRepoCount)
}

func (s *MetricsSuite) TestRepositoryConfigsCount() {
	t := s.T()
	dao := s.dao
	var (
		result int
		err    error
	)

	// The initial state should be 0
	result = dao.RepositoryConfigsCount(context.Background())
	assert.Equal(t, 0, result-s.initialRepositoryConfigsCount)

	// The counter is increased by 1
	repo := &models.Repository{
		URL: "https://www.example.test",
	}
	err = s.tx.Create(repo).Error
	require.NoError(t, err)

	err = s.tx.Create(&models.RepositoryConfiguration{
		Name:                 "test",
		Versions:             pq.StringArray{config.El9},
		Arch:                 config.X8664,
		GpgKey:               "",
		MetadataVerification: false,
		OrgID:                accountIdTest,
		RepositoryUUID:       repo.UUID,
	}).Error
	require.NoError(t, err)

	result = dao.RepositoryConfigsCount(context.Background())
	assert.Equal(t, 1, result-s.initialRepositoryConfigsCount)
}

func (s *MetricsSuite) TestPublicRepositoriesNotIntrospectedLas24HoursCount() {
	t := s.T()
	tx := s.tx
	var (
		result IntrospectionCount
		err    error
		repo   models.Repository
	)
	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, true)
	assert.Equal(t, int64(0), result.Missed-s.initialPublicRepositoriesIntrospectionCount.Missed)

	// This repository won't be counted for the metrics
	repo = models.Repository{
		URL:                          "https://www.example.test",
		Public:                       true,
		LastIntrospectionStatus:      config.StatusPending,
		LastIntrospectionTime:        nil,
		LastIntrospectionError:       nil,
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, true)
	assert.Equal(t, int64(0), result.Missed-s.initialPublicRepositoriesIntrospectionCount.Missed)

	lastIntrospectionTime := time.Now().Add(-37 * time.Hour)
	repo = models.Repository{
		URL:                          "https://www.example2.test",
		Public:                       true,
		LastIntrospectionStatus:      config.StatusInvalid,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, true)
	assert.Equal(t, int64(1), result.Missed-s.initialPublicRepositoriesIntrospectionCount.Missed)

	repo = models.Repository{
		URL:                          "https://www.example3.test",
		Public:                       true,
		LastIntrospectionStatus:      config.StatusUnavailable,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, true)
	assert.Equal(t, int64(2), result.Missed-s.initialPublicRepositoriesIntrospectionCount.Missed)
}

func (s *MetricsSuite) TestPublicRepositoriesFailedIntrospectionCount() {
	t := s.T()
	var (
		result int
		err    error
		repo   models.Repository
	)
	result = s.dao.PublicRepositoriesFailedIntrospectionCount(context.Background())
	assert.Equal(t, 0, result-s.initialPublicRepositoriesFailedIntrospectionCount)

	lastIntrospectionTime := time.Now().Add(-37 * time.Hour)
	repo = models.Repository{
		URL:                          "https://www.example3.test",
		Public:                       true,
		LastIntrospectionStatus:      config.StatusInvalid,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = s.tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.PublicRepositoriesFailedIntrospectionCount(context.Background())
	assert.Equal(t, 1, result-s.initialPublicRepositoriesFailedIntrospectionCount)
}

func (s *MetricsSuite) TestNonPublicRepositoriesNonIntrospectedLast24HoursCount() {
	t := s.T()
	var (
		result IntrospectionCount
		err    error
		repo   models.Repository
	)

	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, false)
	assert.Equal(t, int64(0), result.Missed-s.initialCustomRepositoriesIntrospectionCount.Missed)

	lastIntrospectionTime := time.Now().Add(-38 * time.Hour)
	repo = models.Repository{
		URL:                          "https://www.example4.test",
		Public:                       false,
		LastIntrospectionStatus:      config.StatusInvalid,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = s.tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, false)
	assert.Equal(t, int64(1), result.Missed-s.initialCustomRepositoriesIntrospectionCount.Missed)
}
