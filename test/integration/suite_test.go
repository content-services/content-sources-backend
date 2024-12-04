package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	log2 "log"
	"net/http"
	"os"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/tasks/worker"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	log "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Suite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
	dao                       *dao.DaoRegistry
	taskClient                client.TaskClient
	queue                     queue.PgQueue
	cancel                    context.CancelFunc
}

func (s *Suite) TearDownTest() {
	// Rollback and reset db.DB
	s.cancel()
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

func (s *Suite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.skipDefaultTransactionOld = db.DB.SkipDefaultTransaction
	s.db = db.DB.Session(&gorm.Session{
		SkipDefaultTransaction: false,
		Logger: logger.New(
			log2.New(os.Stderr, "\r\n", log2.LstdFlags),
			logger.Config{
				LogLevel: logger.Info,
			}),
	})
	s.tx = s.db.Begin()
	s.dao = dao.GetDaoRegistry(db.DB)

	wkrCtx, cancel := context.WithCancel(context.Background())

	wkrQueue, err := queue.NewPgQueue(wkrCtx, db.GetUrl())
	require.NoError(s.T(), err)
	s.queue = wkrQueue
	s.taskClient = client.NewTaskClient(&s.queue)

	wrk := worker.NewTaskWorkerPool(&wkrQueue, nil)
	wrk.RegisterHandler(config.IntrospectTask, tasks.IntrospectHandler)
	wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
	wrk.RegisterHandler(config.DeleteSnapshotsTask, tasks.DeleteSnapshotsHandler)
	wrk.RegisterHandler(config.DeleteRepositorySnapshotsTask, tasks.DeleteRepositorySnapshotsHandler)
	wrk.RegisterHandler(config.DeleteTemplatesTask, tasks.DeleteTemplateHandler)
	wrk.RegisterHandler(config.UpdateTemplateContentTask, tasks.UpdateTemplateContentHandler)
	wrk.RegisterHandler(config.UpdateRepositoryTask, tasks.UpdateRepositoryHandler)
	wrk.RegisterHandler(config.AddUploadsTask, tasks.AddUploadsHandler)
	wrk.RegisterHandler(config.UpdateLatestSnapshotTask, tasks.UpdateLatestSnapshotHandler)
	wrk.HeartbeatListener()

	s.cancel = cancel
	go (wrk).StartWorkers(wkrCtx)
	go func() {
		<-wkrCtx.Done()
		wrk.Stop()
		wkrQueue.Close()
	}()
}

func (s *Suite) createAndSyncRepository(orgID string, url string) api.RepositoryResponse {
	// Setup the repository
	repo, err := s.dao.RepositoryConfig.Create(context.Background(), api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr(url),
		AccountID: utils.Ptr(orgID),
		OrgID:     utils.Ptr(orgID),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	s.snapshotAndWait(s.taskClient, repo, repoUuid, orgID)
	return repo
}

func (s *Suite) snapshotAndWait(taskClient client.TaskClient, repo api.RepositoryResponse, repoUuid uuid2.UUID, orgId string) {
	var err error
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()), ObjectType: utils.Ptr(config.ObjectTypeRepository)})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(context.Background(), repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(1 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%v/repodata/repomd.xml",
		snaps.Data[0].URL)
	err = s.getRequest(distPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *Suite) updateTemplateContentAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) payloads.UpdateTemplateContentPayload {
	var err error
	payload := payloads.UpdateTemplateContentPayload{
		TemplateUUID:    tempUUID,
		RepoConfigUUIDs: repoConfigUUIDS,
	}
	task := queue.Task{
		Typename: config.UpdateTemplateContentTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)

	err = json.Unmarshal(taskInfo.Payload, &payload)
	assert.NoError(s.T(), err)

	return payload
}

func (s *Suite) WaitOnTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	if taskInfo.Error != nil {
		// if there is an error, throw and assertion so the error gets printed
		assert.Empty(s.T(), *taskInfo.Error)
	}
	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}

func (s *Suite) waitOnTask(taskUUID uuid2.UUID) *models.TaskInfo {
	// Poll until the task is complete
	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)
	for {
		if taskInfo.Status == config.TaskStatusRunning || taskInfo.Status == config.TaskStatusPending {
			log.Logger.Error().Msg("SLEEPING")
			time.Sleep(1 * time.Second)
		} else {
			break
		}
		taskInfo, err = s.queue.Status(taskUUID)
		assert.NoError(s.T(), err)
	}
	return taskInfo
}

func (s *Suite) getRequest(url string, id identity.Identity, expectedCode int) error {
	client := http.Client{Transport: loggingTransport{}}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	js, err := json.Marshal(identity.XRHID{Identity: id})
	if err != nil {
		return err
	}
	req.Header = http.Header{}
	req.Header.Add(api.IdentityHeader, base64.StdEncoding.EncodeToString(js))
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	assert.Equal(s.T(), expectedCode, res.StatusCode)

	return nil
}
