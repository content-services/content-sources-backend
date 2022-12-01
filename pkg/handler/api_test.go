package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	RegisterPing(router)
	RegisterRoutes(router)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func TestPing(t *testing.T) {
	paths := []string{"/ping", "/ping/"}
	for _, path := range paths {
		req, _ := http.NewRequest("GET", path, nil)
		code, body, err := serveRouter(req)
		assert.Nil(t, err)
		assert.Equal(t, http.StatusOK, code)

		expected := "{\"message\":\"pong\"}\n"
		assert.Equal(t, expected, string(body))
	}
}

func TestPingV1IsNotAvailable(t *testing.T) {
	paths := []string{
		fullRootPath() + "/ping",
		fullRootPath() + "/ping/",
		majorRootPath() + "/ping",
		majorRootPath() + "/ping/",
	}
	for _, path := range paths {
		t.Log(path)
		req, _ := http.NewRequest("GET", path, nil)
		code, body, err := serveRouter(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, code)

		expected := "{\"errors\":[{\"status\":404,\"detail\":\"Not Found\"}]}\n"
		assert.Equal(t, expected, string(body))
	}
}

func TestOpenapi(t *testing.T) {
	req, _ := http.NewRequest("GET", "/api/"+config.DefaultAppName+"/v1.0/openapi.json", nil)
	code, body, err := serveRouter(req)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	js := json.RawMessage{}
	err = json.Unmarshal(body, &js)
	assert.Nil(t, err)
}

func getTestContext(params string) echo.Context {
	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/repositories/"+params, nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec)
}

func TestRootRoute(t *testing.T) {
	assert.Equal(t, fullRootPath(), "/api/"+config.DefaultAppName+"/v1.0")
}

func TestParsePagination(t *testing.T) {
	pageInfo := ParsePagination(getTestContext(""))
	assert.Equal(t, DefaultLimit, pageInfo.Limit)
	assert.Equal(t, 0, pageInfo.Offset)

	pageInfo = ParsePagination(getTestContext("?limit=37&offset=123"))
	assert.Equal(t, 37, pageInfo.Limit)
	assert.Equal(t, 123, pageInfo.Offset)

	pageInfo = ParsePagination(getTestContext("?sort_by[]=status&sort_by[]=url:asc&sort_by[]=name:desc"))
	assert.Equal(t, "status,url:asc,name:desc", pageInfo.SortBy)

	pageInfo = ParsePagination(getTestContext("?sort_by=status"))
	assert.Equal(t, "status", pageInfo.SortBy)
}

func TestCollectionResponse(t *testing.T) {
	coll := api.RepositoryCollectionResponse{}

	setCollectionResponseMetadata(&coll, getTestContext("?offset=0&limit=1"), 10)
	assert.Equal(t, 0, coll.Meta.Offset)
	assert.NotEmpty(t, coll.Links.First)
	assert.NotEmpty(t, coll.Links.Last)
	assert.Empty(t, coll.Links.Prev)
	assert.NotEmpty(t, coll.Links.Next)

	setCollectionResponseMetadata(&coll, getTestContext("?offset=10&limit=1"), 10)
	assert.NotEmpty(t, coll.Links.First)
	assert.NotEmpty(t, coll.Links.Last)
	assert.NotEmpty(t, coll.Links.Prev)
	assert.Empty(t, coll.Links.Next)

	setCollectionResponseMetadata(&coll, getTestContext("?offset=5&limit=1"), 10)
	assert.NotEmpty(t, coll.Links.First)
	assert.NotEmpty(t, coll.Links.Last)
	assert.NotEmpty(t, coll.Links.Prev)
	assert.NotEmpty(t, coll.Links.Next)
}

func TestCreateLink(t *testing.T) {
	link := createLink(getTestContext(""), 99)
	assert.Equal(t, "/api/"+config.DefaultAppName+"/v1.0/repositories/?limit=100&offset=99", link)
}
