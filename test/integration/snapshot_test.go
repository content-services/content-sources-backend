package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	Suite
	dao *dao.DaoRegistry
	ctx context.Context
}

func (s *SnapshotSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background() // Test Context

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.CustomRepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) TestSnapshotUpload() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		AccountID: utils.Ptr(accountId),
		OrgID:     utils.Ptr(accountId),
		Snapshot:  utils.Ptr(true),
		Origin:    utils.Ptr(config.OriginUpload),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)
	s.snapshotAndWait(taskClient, repo, repoUuid, accountId)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/repodata/repomd.xml", snaps.Data[0].URL)
	err = s.getRequest(distPath, identity.Identity{OrgID: accountId, Internal: identity.Internal{OrgID: accountId}}, 200)
	assert.NoError(s.T(), err)
}

func (s *SnapshotSuite) TestSnapshot() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: utils.Ptr(accountId),
		OrgID:     utils.Ptr(accountId),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)
	s.snapshotAndWait(taskClient, repo, repoUuid, accountId)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/repodata/repomd.xml", snaps.Data[0].URL)
	err = s.getRequest(distPath, identity.Identity{OrgID: accountId, Internal: identity.Internal{OrgID: accountId}}, 200)
	assert.NoError(s.T(), err)

	err = s.getRequest(distPath, identity.Identity{X509: &identity.X509{SubjectDN: "warlin.door"}}, 200)
	assert.NoError(s.T(), err)

	// But can't be served without a valid org id or common dn
	_ = s.getRequest(distPath, identity.Identity{}, 403)

	// Update the url
	newUrl := "https://fixtures.pulpproject.org/rpm-with-sha-512/"
	urlUpdated, err := s.dao.RepositoryConfig.Update(s.ctx, accountId, repo.UUID, api.RepositoryUpdateRequest{URL: &newUrl})
	assert.NoError(s.T(), err)
	repo, err = s.dao.RepositoryConfig.Fetch(s.ctx, accountId, repo.UUID)
	assert.NoError(s.T(), err)
	repoUuid, err = uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)
	assert.True(s.T(), urlUpdated)
	assert.NoError(s.T(), err)

	s.snapshotAndWait(taskClient, repo, repoUuid, accountId)

	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, accountId)
	assert.NoError(s.T(), err)

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	remote, err := pulpClient.GetRpmRemoteByName(s.ctx, repo.UUID)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), repo.URL, remote.Url)

	// Create template and add repository to template
	cpClient := candlepin_client.NewCandlepinClient()
	environmentID, templateUUID := s.createTemplate(cpClient, s.ctx, repo, domainName)

	// Delete the template
	deleteTemplateTaskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:   config.DeleteTemplatesTask,
		Payload:    tasks.DeleteTemplatesPayload{TemplateUUID: templateUUID, RepoConfigUUIDs: []string{repo.UUID}},
		OrgId:      repo.OrgID,
		ObjectUUID: utils.Ptr(templateUUID),
		ObjectType: utils.Ptr(config.ObjectTypeTemplate),
	})
	assert.NoError(s.T(), err)

	s.WaitOnTask(deleteTemplateTaskUuid)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:   config.DeleteRepositorySnapshotsTask,
		Payload:    tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID},
		OrgId:      repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was deleted
	snaps, _, err = s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.Error(s.T(), err)
	assert.Empty(s.T(), snaps.Data)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify it's not being served
	err = s.getRequest(distPath, identity.Identity{OrgID: accountId, Internal: identity.Internal{OrgID: accountId}}, 404)
	assert.NoError(s.T(), err)

	// Assert template environment content on longer exists
	content, _ := cpClient.FetchContent(s.ctx, candlepin_client.DevelOrgKey, candlepin_client.GetContentID(repo.UUID))
	assert.Nil(s.T(), content)

	environment, err := cpClient.FetchEnvironment(s.ctx, environmentID)
	assert.Nil(s.T(), err)
	require.Nil(s.T(), environment)
}

type loggingTransport struct{}

func (s loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	bytes, _ := httputil.DumpRequestOut(r, true)

	resp, err := http.DefaultTransport.RoundTrip(r)
	// err is returned after dumping the response

	if resp != nil {
		respBytes, _ := httputil.DumpResponse(resp, true)
		bytes = append(bytes, respBytes...)

		fmt.Printf("%s\n", bytes)
	}
	return resp, err
}

func (s *SnapshotSuite) getRequest(url string, id identity.Identity, expectedCode int) error {
	client := http.Client{Transport: loggingTransport{}}
	req, err := http.NewRequest("GET", url, nil)
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

func (s *SnapshotSuite) TestSnapshotCancel() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: utils.Ptr(accountId),
		OrgID:     utils.Ptr(accountId),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	taskClient := client.NewTaskClient(&s.queue)
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()), ObjectType: utils.Ptr(config.ObjectTypeRepository)})
	assert.NoError(s.T(), err)
	time.Sleep(time.Millisecond * 500)
	s.cancelAndWait(taskClient, taskUuid, repo)
}

func (s *SnapshotSuite) snapshotAndWait(taskClient client.TaskClient, repo api.RepositoryResponse, repoUuid uuid2.UUID, orgId string) {
	var err error
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()), ObjectType: utils.Ptr(config.ObjectTypeRepository)})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/repodata/repomd.xml", snaps.Data[0].URL)
	err = s.getRequest(distPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *SnapshotSuite) cancelAndWait(taskClient client.TaskClient, taskUUID uuid2.UUID, repo api.RepositoryResponse) {
	var err error
	err = taskClient.Cancel(context.Background(), taskUUID.String())
	assert.NoError(s.T(), err)

	s.WaitOnCanceledTask(taskUUID)

	// Verify the snapshot was not created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), api.SnapshotCollectionResponse{Data: []api.SnapshotResponse{}}, snaps)
}

func (s *SnapshotSuite) WaitOnTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	if taskInfo.Error != nil {
		// if there is an error, throw and assertion so the error gets printed
		assert.Empty(s.T(), *taskInfo.Error)
	}
	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}

func (s *SnapshotSuite) WaitOnCanceledTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	require.NotNil(s.T(), taskInfo.Error)
	assert.NotEmpty(s.T(), *taskInfo.Error)
	assert.Equal(s.T(), config.TaskStatusCanceled, taskInfo.Status)
}

func (s *SnapshotSuite) waitOnTask(taskUUID uuid2.UUID) *models.TaskInfo {
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

func (s *SnapshotSuite) createTemplate(cpClient candlepin_client.CandlepinClient, ctx context.Context, repo api.RepositoryResponse, domainName string) (environmentID string, templateUUID string) {
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(repo.OrgID),
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)

	host, err := pulp_client.GetPulpClientWithDomain(domainName).GetContentPath(ctx)
	require.NoError(s.T(), err)

	distPath1 := fmt.Sprintf("%v%v/templates/%v/%v/repodata/repomd.xml", host, domainName, tempResp.UUID, repo.UUID)

	// Update template with new repository
	payload := s.updateTemplateContentAndWait(repo.OrgID, tempResp.UUID, []string{repo.UUID})

	// Verify correct distribution has been created in pulp
	err = s.getRequest(distPath1, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	environmentID = candlepin_client.GetEnvironmentID(payload.TemplateUUID)
	environment, err := cpClient.FetchEnvironment(ctx, environmentID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), environmentID, environment.GetId())
	environmentContent := environment.GetEnvironmentContent()
	require.NotEmpty(s.T(), environmentContent)

	return environmentID, payload.TemplateUUID
}

func (s *SnapshotSuite) updateTemplateContentAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) payloads.UpdateTemplateContentPayload {
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
