package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/tasks/worker"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	zest "github.com/content-services/zest/release/v2024"
	uuid2 "github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UploadSuite struct {
	Suite
	ctx      context.Context
	server   *http.Server
	identity identity.XRHID
	cancel   context.CancelFunc
}

func (s *UploadSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx, s.cancel = context.WithCancel(context.Background())

	config.Get().Features.Snapshots.Enabled = true

	err := db.Connect()
	require.NoError(s.T(), err)

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))

	handler.RegisterRoutes(router)

	s.server = &http.Server{
		Addr:    "127.0.0.1:8100",
		Handler: router,
	}

	// Start a task worker
	go func() {
		pgqueue, err := queue.NewPgQueue(db.GetUrl())
		if err != nil {
			panic(err)
		}
		wrk := worker.NewTaskWorkerPool(&pgqueue, nil)
		wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
		wrk.RegisterHandler(config.AddUploadsTask, tasks.AddUploadsHandler)
		wrk.HeartbeatListener()
		go wrk.StartWorkers(s.ctx)
		<-s.ctx.Done()
		wrk.Stop()
	}()
	// force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
}

func (s *UploadSuite) TearDownTest() {
	s.cancel()
	err := s.server.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	s.Suite.TearDownTest()
}

func (s *UploadSuite) servePulpRouter(req *http.Request) (int, []byte, error) {
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(s.T(), err)

	return response.StatusCode, body, err
}

func TestUploadSuite(t *testing.T) {
	suite.Run(t, new(UploadSuite))
}

func (s *UploadSuite) TestUploadFile() {
	s.identity = test_handler.MockIdentity

	t := s.T()

	size := int64(4)
	uploadResponse := s.CreateUploadRequest(size)

	// Upload a file chunk
	fileContent := []byte(randomFileContent(int(size)))

	sha256sum := s.UploadChunks(fileContent, uploadResponse, size)

	finishResponse := s.finishUpload(uploadResponse, sha256sum)

	// Get artifact href from commit task
	pulpTaskHref := finishResponse.Task

	response := s.fetchTask(pulpTaskHref)
	assert.Equal(t, 1, len(response.CreatedResources))
}

func (s *UploadSuite) TestUploadAndAddRpm() {
	orgId := fmt.Sprintf("UploadandAddRpm-%v", rand.Int())

	// randomize the identity for multiple test runs
	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = orgId

	repo := s.createUploadRepository()

	t := s.T()
	rpm := "./data/giraffe-0.67-2.noarch.rpm"
	stat, err := os.Stat(rpm)
	require.NoError(t, err)

	size := stat.Size()
	uploadResponse := s.CreateUploadRequest(size)

	// Upload a file chunk
	fileContent, err := os.ReadFile(rpm)
	require.NoError(t, err)
	sha256sum := s.UploadChunks(fileContent, uploadResponse, size)

	task := s.addToRepository(repo.UUID, api.AddUploadsRequest{
		Uploads: []api.Upload{{
			Href:   *uploadResponse.PulpHref,
			Sha256: sha256sum,
		}},
	})
	s.waitOnTaskStr(task.UUID)

	repo, err = s.dao.RepositoryConfig.Fetch(context.Background(), repo.OrgID, repo.UUID)
	require.NoError(t, err)
	assert.Equal(t, 1, repo.PackageCount)
}

func (s *UploadSuite) fetchTask(pulpTaskHref string) zest.TaskResponse {
	t := s.T()
	path := api.FullRootPath() + "/pulp/tasks/" + pulpTaskHref
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	response := zest.TaskResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	return response
}

func (s *UploadSuite) finishUpload(uploadResponse zest.UploadResponse, sha256sum string) zest.AsyncOperationResponse {
	t := s.T()
	// Finish/commit an upload
	finishRequest := api.FinishUploadRequest{
		UploadHref: *uploadResponse.PulpHref,
		Sha256:     sha256sum,
	}
	var finishResponse zest.AsyncOperationResponse

	body, err := json.Marshal(finishRequest)
	require.NoError(t, err)

	path := api.FullRootPath() + "/pulp/uploads/" + finishRequest.UploadHref
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	err = json.Unmarshal(body, &finishResponse)
	assert.Nil(t, err)
	assert.NotEmpty(t, finishResponse.Task)
	return finishResponse
}

func (s *UploadSuite) createUploadRepository() api.RepositoryResponse {
	t := s.T()
	repoReq := api.RepositoryRequest{
		Origin:   pointy.Pointer(config.OriginUpload),
		Name:     pointy.Pointer(fmt.Sprintf("upload-repo-%v", rand.Int())),
		Snapshot: pointy.Pointer(true),
	}

	body, err := json.Marshal(repoReq)
	require.NoError(t, err)
	path := api.FullRootPath() + "/repositories/"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")
	code, body, err := s.servePulpRouter(req)
	require.NoError(t, err, "failure creating repo")
	assert.Equal(t, http.StatusCreated, code, string(body))
	repoResp := api.RepositoryResponse{}
	err = json.Unmarshal(body, &repoResp)
	assert.Nil(t, err)
	s.waitOnTaskStr(repoResp.LastSnapshotTaskUUID)

	return repoResp
}

func (s *UploadSuite) addToRepository(repoUUID string, request api.AddUploadsRequest) api.TaskInfoResponse {
	t := s.T()

	body, err := json.Marshal(request)
	require.NoError(t, err)

	path := api.FullRootPath() + "/repositories/" + repoUUID + "/add_uploads/"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusCreated, code)
	csTask := api.TaskInfoResponse{}
	err = json.Unmarshal(body, &csTask)
	assert.Nil(t, err)
	return csTask
}

func (s *UploadSuite) UploadChunks(fileContent []byte, uploadResponse zest.UploadResponse, size int64) string {
	t := s.T()
	// add multipart request
	fileBytes := new(bytes.Buffer)
	multipartWriter := multipart.NewWriter(fileBytes)

	// create sha256 hasher
	hasher := sha256.New()

	// add form field for file and write file content to hasher
	filePart, err := multipartWriter.CreateFormFile("file", "test-rpm-chunk")
	assert.Nil(t, err)
	_, err = filePart.Write(fileContent)
	assert.Nil(t, err)
	hasher.Write(fileContent)
	err = multipartWriter.Close()
	assert.Nil(t, err)

	// calculate checksum of the data written to the hasher
	uploadSha256 := hex.EncodeToString(hasher.Sum(nil))

	path := api.FullRootPath() + "/pulp/uploads/" + *uploadResponse.PulpHref
	req := httptest.NewRequest(http.MethodPut, path, fileBytes)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/*", size-1))

	code, body, err := s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), "pulp_href")
	return uploadSha256
}

func (s *UploadSuite) CreateUploadRequest(size int64) zest.UploadResponse {
	t := s.T()
	// Create an upload
	createRequest := api.CreateUploadRequest{
		Size: size,
	}
	var uploadResponse zest.UploadResponse

	body, err := json.Marshal(createRequest)
	require.NoError(t, err)

	path := api.FullRootPath() + "/pulp/uploads/"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	err = json.Unmarshal(body, &uploadResponse)

	assert.Nil(t, err)
	assert.Contains(t, string(body), "pulp_href")

	return uploadResponse
}

func randomFileContent(size int) string {
	const lookup string = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-"
	return seeds.RandStringWithChars(size, lookup)
}

func (s *UploadSuite) waitOnTaskStr(uuid string) *models.TaskInfo {
	t := s.T()
	taskUUID, err := uuid2.Parse(uuid)
	require.NoError(t, err)
	return s.waitOnTask(taskUUID)
}
