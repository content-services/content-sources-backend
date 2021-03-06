package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	RegisterRoutes(router)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func TestPing(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ping", nil)
	code, body, err := serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	expected := "{\"message\":\"pong\"}\n"
	assert.Equal(t, expected, string(body))
}

func TestPingV1(t *testing.T) {
	print(fullRootPath() + "/ping")
	req, _ := http.NewRequest("GET", majorRootPath()+"/ping", nil)
	code, body, err := serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	expected := "{\"message\":\"pong\"}\n"
	assert.Equal(t, expected, string(body))
}

func TestOpenapi(t *testing.T) {
	req, _ := http.NewRequest("GET", "/api/content_sources/v1.0/openapi.json", nil)
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
	assert.Equal(t, fullRootPath(), "/api/content_sources/v1.0")
}

func TestParsePagination(t *testing.T) {
	pageInfo := ParsePagination(getTestContext(""))
	assert.Equal(t, DefaultLimit, pageInfo.Limit)
	assert.Equal(t, 0, pageInfo.Offset)

	pageInfo = ParsePagination(getTestContext("?limit=37&offset=123"))
	assert.Equal(t, 37, pageInfo.Limit)
	assert.Equal(t, 123, pageInfo.Offset)
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
	assert.Equal(t, "/api/content_sources/v1.0/repositories/?limit=100&offset=99", link)
}
