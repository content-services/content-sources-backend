package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	suite.Suite
	reg        *dao.MockDaoRegistry
	pulpClient *pulp_client.MockPulpClient
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}
func (suite *SnapshotSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.pulpClient = pulp_client.NewMockPulpClient(suite.T())
}

func (suite *SnapshotSuite) serveSnapshotsRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	RegisterSnapshotRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *SnapshotSuite) TestListSnapshotsByDate() {
	t := suite.T()
	repoUUID := "abcadaba"
	request := api.ListSnapshotByDateRequest{Date: "2023-01-22", RepositoryUUIDS: []string{repoUUID}}
	response := []api.ListSnapshotByDateResponse{{RepositoryUUID: repoUUID}}

	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", test_handler.MockOrgId, request).Return(response, nil)

	body, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/snapshots/for_date/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *SnapshotSuite) TestListSnapshotsByDateBadRequestError() {
	t := suite.T()
	RepositoryUUIDS := []string{}

	request := api.ListSnapshotByDateRequest{Date: "2023-01-22", RepositoryUUIDS: RepositoryUUIDS}

	body, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/snapshots/for_date/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *SnapshotSuite) TestListSnapshotsByDateExceedLimitError() {
	t := suite.T()
	RepositoryUUIDS := []string{}
	for i := 0; i < SnapshotByDateQueryLimit+1; i++ {
		RepositoryUUIDS = append(RepositoryUUIDS, seeds.RandomOrgId())
	}

	request := api.ListSnapshotByDateRequest{Date: "2023-01-22", RepositoryUUIDS: RepositoryUUIDS}

	body, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/snapshots/for_date/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
}

func (suite *SnapshotSuite) TestSnapshotList() {
	t := suite.T()

	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	collection := createSnapshotCollection(1, 10, 0)
	repoUUID := "abcadaba"
	suite.reg.Snapshot.WithContextMock().On("List", repoUUID, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	suite.reg.Snapshot.On("WithContext", mock.AnythingOfType("*context.valueCtx")).Return(&suite.reg.Snapshot).Once()
	suite.reg.Snapshot.On("List", test_handler.MockOrgId, repoUUID, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/?limit=%d", api.FullRootPath(), repoUUID, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)

	response := api.SnapshotCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, collection.Data[0].RepositoryPath, response.Data[0].RepositoryPath)
	assert.Equal(t, collection.Data[0].UUID, response.Data[0].UUID)
	assert.Equal(t, collection.Data[0].URL, response.Data[0].URL)
}

func (suite *SnapshotSuite) TestGetRepositoryConfigurationFile() {
	t := suite.T()
	config.Get().Options.ExternalHost = ""
	orgID := test_handler.MockOrgId
	repoUUID := uuid.NewString()
	snapUUID := uuid.NewString()
	repoConfigFile := "file"
	refererHeader := "https://example.com"

	suite.reg.Snapshot.WithContextMock().On("GetRepositoryConfigurationFile", orgID, snapUUID, repoUUID, refererHeader).Return(repoConfigFile, nil).Once()

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s/config.repo", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)

	response := string(body)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, response, repoConfigFile)
}

func (suite *SnapshotSuite) TestGetRepositoryConfigurationFileWithConfig() {
	t := suite.T()
	config.Get().Options.ExternalHost = "http://mycustom.example.com"

	orgID := test_handler.MockOrgId
	repoUUID := uuid.NewString()
	snapUUID := uuid.NewString()
	repoConfigFile := "file"
	refererHeader := "https://example.com"

	// Config overrides it
	suite.reg.Snapshot.WithContextMock().On("GetRepositoryConfigurationFile", orgID, snapUUID, repoUUID, config.Get().Options.ExternalHost).Return(repoConfigFile, nil).Once()

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s/config.repo", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Referer", refererHeader)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)

	response := string(body)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, response, repoConfigFile)
}

func createSnapshotCollection(size, limit, offset int) api.SnapshotCollectionResponse {
	snaps := make([]api.SnapshotResponse, size)
	for i := 0; i < size; i++ {
		snap := api.SnapshotResponse{
			RepositoryPath: "distribution/path/",
			UUID:           uuid.NewString(),
			URL:            "http://pulp-content/pulp/content",
		}
		snaps[i] = snap
	}
	collection := api.SnapshotCollectionResponse{
		Data: snaps,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}
