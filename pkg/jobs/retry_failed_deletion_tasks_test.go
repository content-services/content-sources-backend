package jobs

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type RetryFailedDeletionTasksSuite struct {
	suite.Suite
	db *gorm.DB
	tx *gorm.DB
}

func TestRetryFailedDeletionTasksSuite(t *testing.T) {
	suite.Run(t, new(RetryFailedDeletionTasksSuite))
}

func (s *RetryFailedDeletionTasksSuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB
	s.tx = s.db.Begin()
}

func (s *RetryFailedDeletionTasksSuite) TearDownTest() {
	s.tx.Rollback()
}

func (s *RetryFailedDeletionTasksSuite) createFailedTask(taskType string, retries int, finishedAt time.Time, cancelAttempted bool) *models.TaskInfo {
	// queued_at and started_at must be <= finished_at (DB constraint chronologic_finished_at)
	startedAt := finishedAt.Add(-time.Minute)
	queuedAt := startedAt.Add(-time.Minute)
	task := &models.TaskInfo{
		Id:              uuid.New(),
		Typename:        taskType,
		Status:          config.TaskStatusFailed,
		Queued:          &queuedAt,
		Started:         &startedAt,
		Finished:        &finishedAt,
		OrgId:           "test-org",
		Token:           uuid.New(),
		Retries:         retries,
		CancelAttempted: cancelAttempted,
	}
	err := s.tx.Create(task).Error
	require.NoError(s.T(), err)
	return task
}

func (s *RetryFailedDeletionTasksSuite) TestRetryFailedDeletionTasksQuery() {
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Use very old finished_at values so these rows win ORDER BY finished_at ASC
	// even if the shared DB has other eligible failed deletion tasks.
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now()

	// Should be retried when limit allows: exhausted deletion tasks, oldest first
	task1 := s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries, base.Add(2*time.Hour), false) // newest eligible
	task2 := s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries+1, base.Add(1*time.Hour), false)
	task3 := s.createFailedTask(config.DeleteTemplatesTask, queue.MaxTaskRetries, base, false) // oldest eligible

	// Should not be retried
	s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries-1, base.Add(2*time.Hour), false)
	s.createFailedTask(config.RepositorySnapshotTask, queue.MaxTaskRetries, base.Add(2*time.Hour), false)
	s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries, base.Add(2*time.Hour), true)
	s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries, now.Add(-1*time.Hour), false)

	RetryFailedDeletionTasks([]string{"2"})

	// Oldest two (task3, task2) are selected; task1 is beyond the limit
	var retried []models.TaskInfo
	err := s.tx.Where("id IN ?", []uuid.UUID{task2.Id, task3.Id}).Find(&retried).Error
	require.NoError(s.T(), err)
	require.Len(s.T(), retried, 2)

	for _, task := range retried {
		assert.Equal(s.T(), 0, task.Retries)
		assert.NotNil(s.T(), task.NextRetryTime)
	}

	var notRetried models.TaskInfo
	err = s.tx.First(&notRetried, "id = ?", task1.Id).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), queue.MaxTaskRetries, notRetried.Retries)
	assert.Nil(s.T(), notRetried.NextRetryTime)
}

func (s *RetryFailedDeletionTasksSuite) TestRetryFailedDeletionTasksRespectsLimit() {
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Use very old finished_at values so these rows win ORDER BY finished_at ASC
	// even if the shared DB has other eligible failed deletion tasks.
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	created := make([]*models.TaskInfo, 5)
	for i := 0; i < 5; i++ {
		created[i] = s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries, base.Add(time.Duration(i)*time.Minute), false)
	}
	createdIDs := []uuid.UUID{created[0].Id, created[1].Id, created[2].Id, created[3].Id, created[4].Id}

	RetryFailedDeletionTasks([]string{"3"})

	var tasks []models.TaskInfo
	err := s.tx.Where("id IN ?", createdIDs).Find(&tasks).Error
	require.NoError(s.T(), err)
	require.Len(s.T(), tasks, 5)

	retried := 0
	remaining := 0
	for _, task := range tasks {
		switch {
		case task.Retries == 0 && task.NextRetryTime != nil:
			retried++
			assert.Contains(s.T(), []uuid.UUID{created[0].Id, created[1].Id, created[2].Id}, task.Id)
		case task.Retries >= queue.MaxTaskRetries && task.NextRetryTime == nil:
			remaining++
			assert.Contains(s.T(), []uuid.UUID{created[3].Id, created[4].Id}, task.Id)
		default:
			s.T().Fatalf("unexpected task state for %s: retries=%d next_retry_time=%v", task.Id, task.Retries, task.NextRetryTime)
		}
	}
	assert.Equal(s.T(), 3, retried)
	assert.Equal(s.T(), 2, remaining)
}

func (s *RetryFailedDeletionTasksSuite) TestRetryFailedDeletionTasksSkipsRecentFailures() {
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Very old finished_at so this row wins ORDER BY even with shared DB noise.
	oldTask := s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), false)
	recentTask := s.createFailedTask(config.DeleteRepositorySnapshotsTask, queue.MaxTaskRetries, time.Now().Add(-1*time.Hour), false)

	RetryFailedDeletionTasks([]string{"10"})

	var retried models.TaskInfo
	err := s.tx.First(&retried, "id = ?", oldTask.Id).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 0, retried.Retries)
	assert.NotNil(s.T(), retried.NextRetryTime)

	var recent models.TaskInfo
	err = s.tx.First(&recent, "id = ?", recentTask.Id).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), queue.MaxTaskRetries, recent.Retries)
	assert.Nil(s.T(), recent.NextRetryTime)
}
