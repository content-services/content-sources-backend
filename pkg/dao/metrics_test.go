package dao

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/lib/pq"
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

	_, err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{})
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
		LastIntrospectionError:       utils.Ptr("test"),
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
		LastIntrospectionError:       utils.Ptr("test"),
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
		LastIntrospectionError:       utils.Ptr("test"),
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
		LastIntrospectionError:       utils.Ptr("test"),
		LastIntrospectionUpdateTime:  nil,
		LastIntrospectionSuccessTime: nil,
		PackageCount:                 0,
	}
	err = s.tx.Create(&repo).Error
	require.NoError(t, err)
	result = s.dao.RepositoriesIntrospectionCount(context.Background(), 36, false)
	assert.Equal(t, int64(1), result.Missed-s.initialCustomRepositoriesIntrospectionCount.Missed)
}

func (s *MetricsSuite) TestPendingTasksCount() {
	t := s.T()
	res := s.tx.Create(utils.Ptr(models.TaskInfo{
		Id:       uuid2.New(),
		Token:    uuid2.New(),
		Typename: "TestTaskType",
		Queued:   utils.Ptr(time.Now()),
		Status:   config.TaskStatusPending,
	}))

	assert.NoError(t, res.Error)

	ct := s.dao.PendingTasksCount(context.Background())
	assert.True(t, ct > 0)
}

func (s *MetricsSuite) TestPendingTasksAverageLatency() {
	t := s.T()
	// do to some rounding issues, subtracting 60 seconds seems to result in
	//  a latency of 59.999 seconds
	queued := time.Now().Add(-61 * time.Second)
	res := s.tx.Create(utils.Ptr(models.TaskInfo{
		Id:       uuid2.New(),
		Token:    uuid2.New(),
		Typename: "TestTaskType",
		Queued:   &queued,
		Status:   config.TaskStatusPending,
	}))

	assert.NoError(t, res.Error)
	latency := s.dao.PendingTasksAverageLatency(context.Background())
	assert.True(t, latency >= float64(60))
	assert.True(t, latency < float64(62))
}

func (s *MetricsSuite) TestPendingTasksOldestTask() {
	t := s.T()
	queued := time.Now().Add(-24 * time.Hour)
	task1 := models.TaskInfo{
		Id:       uuid2.New(),
		Token:    uuid2.New(),
		Typename: "TestTaskType",
		Queued:   &queued,
		Status:   config.TaskStatusPending,
	}

	task2 := models.TaskInfo{
		Id:       uuid2.New(),
		Token:    uuid2.New(),
		Typename: "TestTaskType",
		Queued:   utils.Ptr(time.Now()),
		Status:   config.TaskStatusPending,
	}

	res := s.tx.Create(&task1)
	assert.NoError(t, res.Error)
	res = s.tx.Create(&task2)
	assert.NoError(t, res.Error)

	oldestQeuedAt := s.dao.PendingTasksOldestTask(context.Background())
	assert.True(t, oldestQeuedAt > 1)
}

func (s *MetricsSuite) TestRHReposSnapshotNotCompletedInLast36HoursCount() {
	t := s.T()

	initialCount := s.dao.RHReposSnapshotNotCompletedInLast36HoursCount(context.Background())
	assert.NotEqual(t, -1, initialCount)

	rcs, err := seeds.SeedRepositoryConfigurations(s.tx, 4, seeds.SeedOptions{OrgID: config.RedHatOrg})
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rcs))

	r1, r2, r3, r4 := rcs[0], rcs[1], rcs[2], rcs[3]

	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r1.UUID,
		RepoUUID:       r1.RepositoryUUID,
		Typename:       config.RepositorySnapshotTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-10 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-9 * time.Hour)),
		Status:         config.TaskStatusCompleted,
	})
	assert.NoError(t, err)
	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r1.UUID,
		RepoUUID:       r1.RepositoryUUID,
		Typename:       config.IntrospectTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-5 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-4 * time.Hour)),
		Status:         config.TaskStatusFailed,
	})
	assert.NoError(t, err)

	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r2.UUID,
		RepoUUID:       r2.RepositoryUUID,
		Typename:       config.RepositorySnapshotTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-40 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-39 * time.Hour)),
		Status:         config.TaskStatusCompleted,
	})
	assert.NoError(t, err)
	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r2.UUID,
		RepoUUID:       r2.RepositoryUUID,
		Typename:       config.RepositorySnapshotTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-30 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-29 * time.Hour)),
		Status:         config.TaskStatusFailed,
	})
	assert.NoError(t, err)

	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r3.UUID,
		RepoUUID:       r3.RepositoryUUID,
		Typename:       config.RepositorySnapshotTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-30 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-29 * time.Hour)),
		Status:         config.TaskStatusCompleted,
	})
	assert.NoError(t, err)
	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r3.UUID,
		RepoUUID:       r3.RepositoryUUID,
		Typename:       config.RepositorySnapshotTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-20 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-19 * time.Hour)),
		Status:         config.TaskStatusFailed,
	})
	assert.NoError(t, err)

	_, err = seeds.SeedTasks(s.tx, 1, seeds.TaskSeedOptions{
		RepoConfigUUID: r4.UUID,
		RepoUUID:       r4.RepositoryUUID,
		Typename:       config.RepositorySnapshotTask,
		QueuedAt:       utils.Ptr(time.Now().Add(-20 * time.Hour)),
		FinishedAt:     utils.Ptr(time.Now().Add(-19 * time.Hour)),
		Status:         config.TaskStatusFailed,
	})
	assert.NoError(t, err)

	// expecting r2, r4 to be additionally counted in this metric
	count := s.dao.RHReposSnapshotNotCompletedInLast36HoursCount(context.Background())
	assert.Equal(t, 2+initialCount, count)
}

func (s *MetricsSuite) TestTemplatesCount() {
	t := s.T()

	initialCountLatest := s.dao.TemplatesUseLatestCount(context.Background())
	assert.NotEqual(t, -1, initialCountLatest)
	initialCountDate := s.dao.TemplatesUseDateCount(context.Background())
	assert.NotEqual(t, -1, initialCountDate)

	_, err := seeds.SeedTemplates(s.tx, 2, seeds.TemplateSeedOptions{UseLatest: true})
	assert.NoError(t, err)

	_, err = seeds.SeedTemplates(s.tx, 4, seeds.TemplateSeedOptions{})
	assert.NoError(t, err)

	countLatest := s.dao.TemplatesUseLatestCount(context.Background())
	assert.Equal(t, initialCountLatest+2, countLatest)
	countDate := s.dao.TemplatesUseDateCount(context.Background())
	assert.Equal(t, initialCountDate+4, countDate)
}

func (s *MetricsSuite) TestTemplatesUpdatedInLast24Hours() {
	t := s.T()
	tx := s.tx

	initialUpdatedCount := s.dao.TemplatesUpdatedInLast24HoursCount(context.Background())
	assert.NotEqual(t, -1, initialUpdatedCount)

	_, err := seeds.SeedTemplates(s.tx, 2, seeds.TemplateSeedOptions{})
	assert.NoError(t, err)

	t1 := models.Template{
		Base: models.Base{
			UUID:      uuid2.NewString(),
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-12 * time.Hour),
		},
		Name:    seeds.RandStringBytes(10),
		OrgID:   orgIDTest,
		Arch:    config.ANY_ARCH,
		Version: config.ANY_VERSION,
	}
	err = tx.Create(&t1).Error
	require.NoError(t, err)

	t2 := models.Template{
		Base: models.Base{
			UUID:      uuid2.NewString(),
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-48 * time.Hour),
		},
		Name:    seeds.RandStringBytes(10),
		OrgID:   orgIDTest,
		Arch:    config.ANY_ARCH,
		Version: config.ANY_VERSION,
	}
	err = tx.Create(&t2).Error
	require.NoError(t, err)

	updatedCount := s.dao.TemplatesUpdatedInLast24HoursCount(context.Background())
	assert.Equal(t, initialUpdatedCount+3, updatedCount)
}

func (s *MetricsSuite) TestTemplatesAgeAverage() {
	t := s.T()
	tx := s.tx

	initialAgeAverage := s.dao.TemplatesAgeAverage(context.Background())

	t1 := models.Template{
		Base: models.Base{
			UUID:      uuid2.NewString(),
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-12 * time.Hour),
		},
		Name:      seeds.RandStringBytes(10),
		OrgID:     orgIDTest,
		Arch:      config.ANY_ARCH,
		Version:   config.ANY_VERSION,
		UseLatest: false,
		Date:      time.Now().Add(time.Duration(-initialAgeAverage) * 24 * time.Hour).Add(-5 * 24 * time.Hour),
	}
	err := tx.Create(&t1).Error
	require.NoError(t, err)

	updatedAgeAverage := s.dao.TemplatesAgeAverage(context.Background())
	assert.True(t, initialAgeAverage < updatedAgeAverage)

	t2 := models.Template{
		Base: models.Base{
			UUID:      uuid2.NewString(),
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-48 * time.Hour),
		},
		Name:      seeds.RandStringBytes(10),
		OrgID:     orgIDTest,
		Arch:      config.ANY_ARCH,
		Version:   config.ANY_VERSION,
		UseLatest: false,
		Date:      time.Now().Add(time.Duration(-initialAgeAverage) * 24 * time.Hour).Add(10 * 24 * time.Hour),
	}
	err = tx.Create(&t2).Error
	require.NoError(t, err)

	updatedAgeAverage = s.dao.TemplatesAgeAverage(context.Background())
	assert.True(t, initialAgeAverage > updatedAgeAverage)
}

func (s *MetricsSuite) TestTemplatesUpdateTaskPendingAverage() {
	t := s.T()
	tx := s.tx

	queued := time.Now().Add(-61 * time.Second)
	res := tx.Create(utils.Ptr(models.TaskInfo{
		Id:       uuid2.New(),
		Token:    uuid2.New(),
		Typename: config.UpdateTemplateContentTask,
		Queued:   &queued,
		Status:   config.TaskStatusPending,
	}))
	assert.NoError(t, res.Error)

	queued = time.Now().Add(-100 * time.Second)
	res = tx.Create(utils.Ptr(models.TaskInfo{
		Id:       uuid2.New(),
		Token:    uuid2.New(),
		Typename: config.RepositorySnapshotTask,
		Queued:   &queued,
		Status:   config.TaskStatusPending,
	}))
	assert.NoError(t, res.Error)

	pendingTimeAverage := s.dao.TaskPendingTimeAverageByType(context.Background())
	for _, task := range []string{config.UpdateTemplateContentTask, config.RepositorySnapshotTask} {
		value := 0.0
		indexFunc := func(a TaskTypePendingTimeAverage) bool {
			return a.Type == task
		}

		if i := slices.IndexFunc(pendingTimeAverage, indexFunc); i >= 0 {
			value = pendingTimeAverage[i].PendingTime
		}
		assert.NotEqual(t, 0.0, value)

		switch task {
		case config.UpdateTemplateContentTask:
			assert.True(t, value >= float64(60))
			assert.True(t, value < float64(62))
		case config.RepositorySnapshotTask:
			assert.True(t, value >= float64(99))
			assert.True(t, value < float64(101))
		}
	}
}
