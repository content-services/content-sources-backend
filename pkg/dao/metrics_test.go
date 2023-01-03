package dao

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	*DaoSuite

	dao MetricsDao

	initialRepoCount                                            int
	initialRepositoryConfigsCount                               int
	initialPublicRepositoriesNotIntrospectedLas24HoursCount     int
	initialPublicRepositoriesFailedIntrospectionCount           int
	initialNonPublicRepositoriesNonIntrospectedLast24HoursCount int

	// repoConfig  *models.RepositoryConfiguration
	// repoPublic  *models.Repository
	// repoPrivate *models.Repository
}

func (s *MetricsSuite) SetupTest() {
	s.DaoSuite.SetupTest()
	s.dao = GetMetricsDao(s.tx)

	// // Create public repository entry
	// repoPublic := repoPublicTest.DeepCopy()
	// if err := s.tx.Create(&repoPublic).Error; err != nil {
	// 	s.FailNow("Preparing Repository record UUID=" + repoPublic.UUID)
	// }
	// s.repoPublic = repoPublic

	// // Create private repository entry
	// repoPrivate := repoPrivateTest.DeepCopy()
	// if err := s.tx.Create(&repoPrivate).Error; err != nil {
	// 	s.FailNow(err.Error())
	// }
	// s.repoPrivate = repoPrivate

	// // Create repository configuration entry
	// repoConfig := repoConfigTest1.DeepCopy()
	// // repoConfig.Repository = *repoPublic
	// repoConfig.RepositoryUUID = repoPublic.Base.UUID
	// if err := s.tx.Create(&repoConfig).Error; err != nil {
	// 	s.FailNow("Preparing RepositoryConfiguration record UUID=" + repoConfig.UUID)
	// }
	// s.repoConfig = repoConfig

	s.initialRepoCount = s.dao.RepositoriesCount()
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialRepositoryConfigsCount = s.dao.RepositoryConfigsCount()
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialPublicRepositoriesNotIntrospectedLas24HoursCount = s.dao.PublicRepositoriesNotIntrospectedLas24HoursCount()
	if s.tx.Error != nil {
		s.FailNow(s.tx.Error.Error())
	}
	s.initialNonPublicRepositoriesNonIntrospectedLast24HoursCount = s.dao.NonPublicRepositoriesNonIntrospectedLast24HoursCount()
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

func (s *MetricsSuite) TestRepositoriesCount() {
	t := s.T()
	dao := s.dao
	var result int

	// The initial state should be 0
	result = dao.RepositoriesCount()
	assert.Equal(t, 0, result-s.initialRepoCount)

	// The counter is increased by 1
	s.tx.Create(&models.Repository{
		URL:                          "https://",
		Public:                       true,
		Status:                       config.StatusInvalid,
		LastIntrospectionTime:        nil,
		LastIntrospectionSuccessTime: nil,
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionError:       nil,
		PackageCount:                 0,
		Revision:                     "12345",
	})

	result = dao.RepositoriesCount()
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
	result = dao.RepositoryConfigsCount()
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

	result = dao.RepositoryConfigsCount()
	assert.Equal(t, 1, result-s.initialRepositoryConfigsCount)
}

func (s *MetricsSuite) TestPublicRepositoriesNotIntrospectedLas24HoursCount() {
	t := s.T()
	tx := s.tx
	var (
		result int
		err    error
		repo   models.Repository
	)
	result = s.dao.PublicRepositoriesNotIntrospectedLas24HoursCount()
	assert.Equal(t, 0, result-s.initialPublicRepositoriesNotIntrospectedLas24HoursCount)

	// TODO Ask if we want to include the repositories which have not been introspected yet (Pending state)
	repo = models.Repository{
		URL:                          "https://www.example.test",
		Revision:                     "12345",
		Public:                       true,
		Status:                       config.StatusPending,
		LastIntrospectionTime:        nil,
		LastIntrospectionError:       nil,
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.PublicRepositoriesNotIntrospectedLas24HoursCount()
	assert.Equal(t, 1, result-s.initialPublicRepositoriesNotIntrospectedLas24HoursCount)

	lastIntrospectionTime := time.Now().Add(-25 * time.Hour)
	repo = models.Repository{
		URL:                          "https://www.example2.test",
		Revision:                     "12347",
		Public:                       true,
		Status:                       config.StatusInvalid,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  &lastIntrospectionTime,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.PublicRepositoriesNotIntrospectedLas24HoursCount()
	assert.Equal(t, 2, result-s.initialPublicRepositoriesNotIntrospectedLas24HoursCount)
}

func (s *MetricsSuite) TestPublicRepositoriesFailedIntrospectionCount() {
	t := s.T()
	var (
		result int
		err    error
		repo   models.Repository
	)
	result = s.dao.PublicRepositoriesFailedIntrospectionCount()
	assert.Equal(t, 0, result-s.initialPublicRepositoriesFailedIntrospectionCount)

	lastIntrospectionTime := time.Now().Add(-25 * time.Hour)
	repo = models.Repository{
		URL:                          "https://www.example3.test",
		Revision:                     "12347",
		Public:                       true,
		Status:                       config.StatusInvalid,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  &lastIntrospectionTime,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = s.tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.PublicRepositoriesFailedIntrospectionCount()
	assert.Equal(t, 1, result-s.initialPublicRepositoriesFailedIntrospectionCount)
}

func (s *MetricsSuite) TestNonPublicRepositoriesNonIntrospectedLast24HoursCount() {
	t := s.T()
	var (
		result int
		err    error
		repo   models.Repository
	)
	result = s.dao.NonPublicRepositoriesNonIntrospectedLast24HoursCount()
	assert.Equal(t, 0, result-s.initialNonPublicRepositoriesNonIntrospectedLast24HoursCount)

	lastIntrospectionTime := time.Now().Add(-25 * time.Hour)
	repo = models.Repository{
		URL:                          "https://www.example4.test",
		Revision:                     "12347",
		Public:                       false,
		Status:                       config.StatusInvalid,
		LastIntrospectionTime:        &lastIntrospectionTime,
		LastIntrospectionError:       pointy.String("test"),
		LastIntrospectionUpdateTime:  &lastIntrospectionTime,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = s.tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.NonPublicRepositoriesNonIntrospectedLast24HoursCount()
	assert.Equal(t, 1, result-s.initialNonPublicRepositoriesNonIntrospectedLast24HoursCount)
}
