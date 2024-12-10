package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TemplatesSuite struct {
	suite.Suite
	reg    *dao.MockDaoRegistry
	tcMock *client.MockTaskClient
}

func TestTemplatesSuite(t *testing.T) {
	suite.Run(t, new(TemplatesSuite))
}
func (suite *TemplatesSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.tcMock = client.NewMockTaskClient(suite.T())
}

func (suite *TemplatesSuite) serveTemplatesRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	th := RepositoryHandler{
		DaoRegistry: *suite.reg.ToDaoRegistry(),
		TaskClient:  suite.tcMock,
	}

	RegisterTemplateRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &th.TaskClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *TemplatesSuite) TestCreate() {
	orgID := test_handler.MockOrgId
	template := api.TemplateRequest{
		Name:            utils.Ptr("test template"),
		Description:     utils.Ptr("a new template"),
		RepositoryUUIDS: []string{"repo-uuid"},
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
		OrgID:           &orgID,
		User:            utils.Ptr("user"),
	}

	expected := api.TemplateResponse{
		UUID:            "uuid",
		Name:            "test template",
		OrgID:           orgID,
		Description:     "a new template",
		Arch:            config.AARCH64,
		Version:         config.El8,
		Date:            time.Time{},
		RepositoryUUIDS: []string{"repo-uuid"},
	}

	suite.reg.Template.On("Create", test.MockCtx(), template).Return(expected, nil)
	mockUpdateTemplateContentEvent(suite.tcMock, suite, expected.UUID, template.RepositoryUUIDS)

	body, err := json.Marshal(template)
	require.NoError(suite.T(), err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/templates/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)

	var response api.TemplateResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), response.Name)
	assert.Equal(suite.T(), http.StatusCreated, code)
}

func (suite *TemplatesSuite) TestFetch() {
	orgID := test_handler.MockOrgId
	uuid := "uuid"
	expected := api.TemplateResponse{
		UUID:        uuid,
		Name:        "test template",
		OrgID:       orgID,
		Description: "a new template",
		Arch:        config.AARCH64,
		Version:     config.El8,
		Date:        time.Time{},
	}

	suite.reg.Template.On("Fetch", test.MockCtx(), orgID, uuid, false).Return(expected, nil)

	body, err := json.Marshal(expected)
	require.NoError(suite.T(), err)

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/templates/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)

	var response api.TemplateResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), expected.Name, response.Name)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *TemplatesSuite) TestFetchNotFound() {
	orgID := test_handler.MockOrgId
	uuid := "uuid"
	template := api.TemplateResponse{
		UUID:        uuid,
		Name:        "test template",
		OrgID:       orgID,
		Description: "a new template",
		Arch:        config.AARCH64,
		Version:     config.El8,
		Date:        time.Time{},
	}

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Not found",
	}

	suite.reg.Template.On("Fetch", test.MockCtx(), orgID, uuid, false).Return(api.TemplateResponse{}, &daoError)

	body, err := json.Marshal(template)
	require.NoError(suite.T(), err)

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/templates/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, _, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}

func (suite *TemplatesSuite) TestList() {
	orgID := test_handler.MockOrgId
	collection := createTemplateCollection(1, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	suite.reg.Template.On("List", test.MockCtx(), orgID, paginationData, api.TemplateFilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/templates/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)

	response := api.TemplateCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	assert.Equal(suite.T(), collection.Data[0].Name, response.Data[0].Name)
	assert.Equal(suite.T(), collection.Data[0].Version, response.Data[0].Version)
	assert.Equal(suite.T(), collection.Data[0].Arch, response.Data[0].Arch)
	assert.Equal(suite.T(), collection.Data[0].Description, response.Data[0].Description)
	assert.Equal(suite.T(), collection.Data[0].OrgID, response.Data[0].OrgID)
	assert.Equal(suite.T(), collection.Data[0].UUID, response.Data[0].UUID)
}

func (suite *TemplatesSuite) TestListNoTemplates() {
	t := suite.T()

	collection := api.TemplateCollectionResponse{}
	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TemplateFilterData{}).Return(collection, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/templates/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TemplateCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(0), response.Meta.Count)
	assert.Equal(t, 100, response.Meta.Limit)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, api.FullRootPath()+"/templates/?limit=100&offset=0", response.Links.Last)
	assert.Equal(t, api.FullRootPath()+"/templates/?limit=100&offset=0", response.Links.First)
}

func (suite *TemplatesSuite) TestTemplatePagedExtraRemaining() {
	t := suite.T()

	collection := api.TemplateCollectionResponse{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}

	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, api.TemplateFilterData{}).Return(collection, int64(102), nil).Once()
	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, api.TemplateFilterData{}).Return(collection, int64(102), nil).Once()

	path := fmt.Sprintf("%s/templates/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TemplateCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(102), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TemplateCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *TemplatesSuite) TestListWithFilters() {
	t := suite.T()
	collection := api.TemplateCollectionResponse{}

	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 100}, api.TemplateFilterData{Name: "template", Arch: "x86_64",
		RepositoryUUIDs: []string{"abcd", "efgh"}}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/templates/?name=%v&arch=%v&repository_uuids=%v", api.FullRootPath(), "template", "x86_64", "abcd,efgh")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, _, err := suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *TemplatesSuite) TestListPagedNoRemaining() {
	t := suite.T()

	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}

	collection := api.TemplateCollectionResponse{}
	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, api.TemplateFilterData{}).Return(collection, int64(100), nil)
	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, api.TemplateFilterData{}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/templates/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TemplateCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TemplateCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *TemplatesSuite) TestListDaoError() {
	t := suite.T()

	daoError := ce.DaoError{
		Message: "Column doesn't exist",
	}
	paginationData := api.PaginationData{Limit: DefaultLimit}

	suite.reg.Template.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TemplateFilterData{}).
		Return(api.TemplateCollectionResponse{}, int64(0), &daoError)

	path := fmt.Sprintf("%s/templates/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *TemplatesSuite) TestDelete() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	uuid := "valid-uuid"
	expected := api.TemplateResponse{
		UUID:        uuid,
		Name:        "test template",
		OrgID:       orgID,
		Description: "a new template",
		Arch:        config.AARCH64,
		Version:     config.El8,
		Date:        time.Time{},
	}

	suite.reg.Template.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid, false).Return(expected, nil)

	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuid, config.UpdateTemplateContentTask).Return([]string{"task-uuid"}, nil)
	suite.tcMock.On("Cancel", test.MockCtx(), "task-uuid").Return(nil)

	_, err := json.Marshal(expected)
	require.NoError(suite.T(), err)

	suite.reg.Template.On("SoftDelete", test.MockCtx(), test_handler.MockOrgId, uuid).Return(nil)
	mockTemplateDeleteEvent(suite.tcMock, uuid)

	req := httptest.NewRequest(http.MethodDelete, api.FullRootPath()+"/templates/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveTemplatesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func createTemplateCollection(size, limit, offset int) api.TemplateCollectionResponse {
	templates := make([]api.TemplateResponse, size)
	for i := 0; i < size; i++ {
		repo := api.TemplateResponse{
			UUID:    fmt.Sprintf("%d", i),
			Name:    fmt.Sprintf("repo_%d", i),
			Version: config.El7,
			Arch:    config.X8664,
			OrgID:   test_handler.MockOrgId,
		}
		templates[i] = repo
	}
	collection := api.TemplateCollectionResponse{
		Data: templates,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}

func mockTemplateDeleteEvent(tcMock *client.MockTaskClient, templateUUID string) {
	tcMock.On("Enqueue", queue.Task{
		Typename:   config.DeleteTemplatesTask,
		Payload:    tasks.DeleteTemplatesPayload{TemplateUUID: templateUUID},
		OrgId:      test_handler.MockOrgId,
		AccountId:  test_handler.MockAccountNumber,
		ObjectUUID: utils.Ptr(templateUUID),
		ObjectType: utils.Ptr(config.ObjectTypeTemplate),
	}).Return(nil, nil)
}

func mockUpdateTemplateContentEvent(tcMock *client.MockTaskClient, templatesSuite *TemplatesSuite, templateUUID string, repoConfigUUIDs []string) {
	id := uuid.New()
	if config.Get().Clients.Candlepin.Server != "" {
		tcMock.On("Enqueue", queue.Task{
			Typename:   config.UpdateTemplateContentTask,
			ObjectType: utils.Ptr(config.ObjectTypeTemplate),
			ObjectUUID: utils.Ptr(templateUUID),
			Payload: payloads.UpdateTemplateContentPayload{
				TemplateUUID:    templateUUID,
				RepoConfigUUIDs: repoConfigUUIDs,
			},
			OrgId:     test_handler.MockOrgId,
			AccountId: test_handler.MockAccountNumber,
			Priority:  1,
		}).Return(id, nil)
		templatesSuite.reg.Template.On(
			"UpdateLastUpdateTask",
			test.MockCtx(),
			id.String(),
			test_handler.MockOrgId,
			templateUUID,
		).Return(nil)
	}
}

func (suite *TemplatesSuite) TestPartialUpdate() {
	uuid := "uuid"
	orgID := test_handler.MockOrgId
	template := api.TemplateUpdateRequest{
		Description:     utils.Ptr("a new template"),
		RepositoryUUIDS: []string{"repo-uuid"},
		OrgID:           &orgID,
		Name:            utils.Ptr("test template"),
	}

	expected := api.TemplateResponse{
		UUID:            "uuid",
		Name:            "test template",
		OrgID:           orgID,
		Description:     "a new template",
		Arch:            config.AARCH64,
		Version:         config.El8,
		Date:            time.Time{},
		RepositoryUUIDS: []string{"repo-uuid"},
	}

	suite.reg.Template.On("Update", test.MockCtx(), orgID, uuid, template).Return(expected, nil)
	mockUpdateTemplateContentEvent(suite.tcMock, suite, expected.UUID, template.RepositoryUUIDS)

	body, err := json.Marshal(template)
	require.NoError(suite.T(), err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/templates/uuid",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)

	var response api.TemplateResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), response.Name)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *TemplatesSuite) TestPartialUpdateEmptyDate() {
	uuid := "uuid"
	orgID := test_handler.MockOrgId
	template := api.TemplateUpdateRequest{
		UseLatest: utils.Ptr(true),
		Date:      utils.Ptr(api.EmptiableDate(time.Time{})),
		OrgID:     &orgID,
		User:      utils.Ptr("user"),
	}

	expected := api.TemplateResponse{
		UUID:      "uuid",
		OrgID:     orgID,
		Date:      time.Time{},
		UseLatest: true,
	}

	suite.reg.Template.On("Update", test.MockCtx(), orgID, uuid, template).Return(expected, nil)
	mockUpdateTemplateContentEvent(suite.tcMock, suite, expected.UUID, template.RepositoryUUIDS)

	body := []byte(fmt.Sprintf(`{"date": "", "use_latest": true, "org_id": "%s"}`, orgID))

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/templates/uuid",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)

	var response api.TemplateResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.Equal(suite.T(), time.Time{}, response.Date)
}

func (suite *TemplatesSuite) TestFullUpdate() {
	uuid := "uuid"
	orgID := test_handler.MockOrgId
	template := api.TemplateUpdateRequest{
		Description: utils.Ptr("Some desc"),
		Date:        utils.Ptr(api.EmptiableDate(time.Time{})),
		Name:        utils.Ptr("Some name"),
	}
	templateExpected := api.TemplateUpdateRequest{
		Description: template.Description,
		Date:        utils.Ptr(api.EmptiableDate(time.Time{})),
		Name:        template.Name,
	}
	templateExpected.FillDefaults()

	expected := api.TemplateResponse{
		UUID:            "uuid",
		Name:            *templateExpected.Name,
		OrgID:           orgID,
		Description:     *templateExpected.Description,
		Arch:            config.AARCH64,
		Version:         config.El8,
		Date:            time.Time{},
		RepositoryUUIDS: []string{},
	}

	suite.reg.Template.On("Update", test.MockCtx(), orgID, uuid, templateExpected).Return(expected, nil)
	mockUpdateTemplateContentEvent(suite.tcMock, suite, expected.UUID, expected.RepositoryUUIDS)

	body, err := json.Marshal(template)
	require.NoError(suite.T(), err)

	req := httptest.NewRequest(http.MethodPut, api.FullRootPath()+"/templates/uuid",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveTemplatesRouter(req)
	assert.Nil(suite.T(), err)

	var response api.TemplateResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), response.Name)
	assert.Equal(suite.T(), http.StatusOK, code)
}
