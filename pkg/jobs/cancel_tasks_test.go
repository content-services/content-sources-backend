package jobs

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type CancelTasksSuite struct {
	suite.Suite
	db *gorm.DB
	tx *gorm.DB
}

func TestCancelTasksSuite(t *testing.T) {
	suite.Run(t, new(CancelTasksSuite))
}

func (s *CancelTasksSuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB
	s.tx = s.db.Begin()
}

func (s *CancelTasksSuite) TearDownTest() {
	s.tx.Rollback()
}

func (s *CancelTasksSuite) createTask(taskType string, status string, queuedAt time.Time) *models.TaskInfo {
	task := &models.TaskInfo{
		Id:       uuid.New(),
		Typename: taskType,
		Status:   status,
		Queued:   &queuedAt,
		OrgId:    "test-org",
		Token:    uuid.New(), // Required: token cannot be nil UUID
	}
	err := s.tx.Create(task).Error
	require.NoError(s.T(), err)
	return task
}

func (s *CancelTasksSuite) TestQueryLogic_WithTimeConstraint() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Test canceling tasks queued in the last 3 hours
	cutoffTime := time.Now().Add(-3 * time.Hour)

	// Tasks that should match (within time window, correct type, pending/running)
	task1 := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusPending, time.Now().Add(-2*time.Hour))
	task2 := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusRunning, time.Now().Add(-30*time.Minute))
	taskAtCutoff := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusPending, cutoffTime)

	// Tasks that should NOT match
	taskTooOld := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusPending, time.Now().Add(-4*time.Hour))
	taskJustBeforeCutoff := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusPending, cutoffTime.Add(-1*time.Second))
	taskWrongType := s.createTask(config.RepositorySnapshotTask, config.TaskStatusPending, time.Now().Add(-2*time.Hour))
	taskFailed := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusFailed, time.Now().Add(-1*time.Hour))

	startedTime := time.Now().Add(-1 * time.Hour)
	finishedTime := time.Now()
	taskCompleted := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusCompleted, time.Now().Add(-1*time.Hour))
	taskCompleted.Started = &startedTime
	taskCompleted.Finished = &finishedTime
	s.tx.Save(taskCompleted)

	// Query for tasks that would be canceled
	var tasks []models.TaskInfo
	result := s.tx.Where("type = ? AND status in (?) AND finished_at IS NULL AND queued_at >= ?",
		config.DeleteRepositorySnapshotsTask,
		[]string{config.TaskStatusPending, config.TaskStatusRunning},
		cutoffTime).
		Find(&tasks)

	require.NoError(s.T(), result.Error)
	assert.Len(s.T(), tasks, 3, "Should find exactly 3 tasks")

	taskIDs := make([]uuid.UUID, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.Id
	}

	// Verify correct tasks found
	assert.Contains(s.T(), taskIDs, task1.Id)
	assert.Contains(s.T(), taskIDs, task2.Id)
	assert.Contains(s.T(), taskIDs, taskAtCutoff.Id)

	// Verify excluded tasks
	assert.NotContains(s.T(), taskIDs, taskTooOld.Id)
	assert.NotContains(s.T(), taskIDs, taskJustBeforeCutoff.Id)
	assert.NotContains(s.T(), taskIDs, taskWrongType.Id)
	assert.NotContains(s.T(), taskIDs, taskFailed.Id)
	assert.NotContains(s.T(), taskIDs, taskCompleted.Id)
}

func (s *CancelTasksSuite) TestQueryLogic_NoTimeConstraint() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Test with no time constraint - should find all pending/running tasks regardless of queued time
	// Also verifies only pending and running statuses are matched

	// Create tasks at various times with different statuses
	taskOld := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusPending, time.Now().Add(-24*time.Hour))
	taskRecent := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusRunning, time.Now().Add(-10*time.Minute))
	taskMedium := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusPending, time.Now().Add(-5*time.Hour))

	// Should NOT match - wrong statuses
	taskCompleted := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusCompleted, time.Now().Add(-2*time.Hour))
	taskFailed := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusFailed, time.Now().Add(-2*time.Hour))
	taskCanceled := s.createTask(config.DeleteRepositorySnapshotsTask, config.TaskStatusCanceled, time.Now().Add(-2*time.Hour))

	// Should NOT match - wrong type
	taskWrongType := s.createTask(config.RepositorySnapshotTask, config.TaskStatusPending, time.Now().Add(-2*time.Hour))

	// Query for tasks that would be canceled (no time constraint)
	var tasks []models.TaskInfo
	result := s.tx.Where("type = ? AND status in (?) AND finished_at IS NULL",
		config.DeleteRepositorySnapshotsTask,
		[]string{config.TaskStatusPending, config.TaskStatusRunning}).
		Find(&tasks)

	require.NoError(s.T(), result.Error)
	assert.Len(s.T(), tasks, 3, "Should find all pending/running tasks regardless of time")

	taskIDs := make([]uuid.UUID, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.Id
	}

	// Verify all pending/running tasks found
	assert.Contains(s.T(), taskIDs, taskOld.Id)
	assert.Contains(s.T(), taskIDs, taskRecent.Id)
	assert.Contains(s.T(), taskIDs, taskMedium.Id)

	// Verify excluded tasks
	assert.NotContains(s.T(), taskIDs, taskCompleted.Id)
	assert.NotContains(s.T(), taskIDs, taskFailed.Id)
	assert.NotContains(s.T(), taskIDs, taskCanceled.Id)
	assert.NotContains(s.T(), taskIDs, taskWrongType.Id)
}
