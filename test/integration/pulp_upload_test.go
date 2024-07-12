package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UploadSuite struct {
	Suite
	ctx    context.Context
	server *http.Server
}

func (s *UploadSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background()

	err := db.Connect()
	require.NoError(s.T(), err)

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	pathPrefix := router.Group(api.FullRootPath())

	handler.RegisterPulpRoutes(pathPrefix, s.dao)

	s.server = &http.Server{
		Addr:    "127.0.0.1:8000",
		Handler: router,
	}

	// start the server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			log.Fatal().Msg("Could not start server")
		}
	}()

	// force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
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

func (s *UploadSuite) TestUpload() {
	t := s.T()

	// Create an upload
	createRequest := api.CreateUploadRequest{
		Size: 4,
	}
	var uploadResponse zest.UploadResponse

	body, err := json.Marshal(createRequest)
	require.NoError(t, err)

	path := api.FullRootPath() + "/pulp/uploads/"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	err = json.Unmarshal(body, &uploadResponse)
	assert.Nil(t, err)
	assert.Contains(t, string(body), "pulp_href")

	// Upload a file chunk

	// add multipart request
	fileBytes := new(bytes.Buffer)
	multipartWriter := multipart.NewWriter(fileBytes)

	// create sha256 hasher
	hasher := sha256.New()

	// add form field for file and write file content to hasher
	filePart, err := multipartWriter.CreateFormFile("file", "test-rpm-chunk")
	assert.Nil(t, err)
	fileContent := []byte(randomFileContent(int(createRequest.Size)))
	_, err = filePart.Write(fileContent)
	assert.Nil(t, err)
	hasher.Write(fileContent)
	err = multipartWriter.Close()
	assert.Nil(t, err)

	// calculate checksum of the data written to the hasher
	uploadSha256 := hex.EncodeToString(hasher.Sum(nil))

	path = api.FullRootPath() + "/pulp/uploads/" + *uploadResponse.PulpHref
	req = httptest.NewRequest(http.MethodPut, path, fileBytes)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/*", createRequest.Size-1))

	code, body, err = s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), "pulp_href")

	// Finish/commit an upload

	finishRequest := api.FinishUploadRequest{
		UploadHref: *uploadResponse.PulpHref,
		Sha256:     uploadSha256,
	}
	var finishResponse zest.AsyncOperationResponse

	body, err = json.Marshal(finishRequest)
	require.NoError(t, err)

	path = api.FullRootPath() + "/pulp/uploads/" + finishRequest.UploadHref
	req = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, body, err = s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	err = json.Unmarshal(body, &finishResponse)
	assert.Nil(t, err)
	assert.NotEmpty(t, finishResponse.Task)

	// Get artifact href from commit task

	pulpTaskHref := finishResponse.Task

	path = api.FullRootPath() + "/pulp/tasks/" + pulpTaskHref
	req = httptest.NewRequest(http.MethodGet, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, body, err = s.servePulpRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	response := zest.TaskResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.CreatedResources))
}

func randomFileContent(size int) string {
	const lookup string = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-"
	return seeds.RandStringWithChars(size, lookup)
}
