package handler

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveRouter(req *http.Request) (int, []byte, error) {
	err := config.ConfigureTang()
	if err != nil {
		return 0, nil, err
	}
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	RegisterPing(router)
	RegisterRoutes(context.Background(), router)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func TestPing(t *testing.T) {
	PulpConnected = true
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
		api.FullRootPath() + "/ping",
		api.FullRootPath() + "/ping/",
		api.MajorRootPath() + "/ping",
		api.MajorRootPath() + "/ping/",
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
	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repositories/"+params, nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec)
}

func TestRootRoute(t *testing.T) {
	assert.Equal(t, api.FullRootPath(), "/api/"+config.DefaultAppName+"/v1.0")
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

	pageInfo = ParsePagination(getTestContext("?limit=0"))
	assert.Equal(t, DefaultLimit, pageInfo.Limit)
}

func TestParsePaginationWithMaxLimit(t *testing.T) {
	pageInfo := ParsePaginationWithMaxLimit(getTestContext("?limit=1000"), 1000)
	assert.Equal(t, 1000, pageInfo.Limit)

	pageInfo = ParsePaginationWithMaxLimit(getTestContext("?limit=1001"), 1000)
	assert.Equal(t, 1000, pageInfo.Limit)

	pageInfo = ParsePagination(getTestContext("?limit=1000"))
	assert.Equal(t, MaxLimit, pageInfo.Limit)
}

func TestCollectionResponse(t *testing.T) {
	coll := api.RepositoryCollectionResponse{}

	SetCollectionResponseMetadata(&coll, getTestContext("?offset=0&limit=1"), 10)
	assert.Equal(t, 0, coll.Meta.Offset)
	assert.NotEmpty(t, coll.Links.First)
	assert.NotEmpty(t, coll.Links.Last)
	assert.Empty(t, coll.Links.Prev)
	assert.NotEmpty(t, coll.Links.Next)

	SetCollectionResponseMetadata(&coll, getTestContext("?offset=10&limit=1"), 10)
	assert.NotEmpty(t, coll.Links.First)
	assert.NotEmpty(t, coll.Links.Last)
	assert.NotEmpty(t, coll.Links.Prev)
	assert.Empty(t, coll.Links.Next)

	SetCollectionResponseMetadata(&coll, getTestContext("?offset=5&limit=1"), 10)
	assert.NotEmpty(t, coll.Links.First)
	assert.NotEmpty(t, coll.Links.Last)
	assert.NotEmpty(t, coll.Links.Prev)
	assert.NotEmpty(t, coll.Links.Next)
}

func TestCreateLink(t *testing.T) {
	link := createLink(getTestContext(""), DefaultLimit, 99)
	assert.Equal(t, "/api/"+config.DefaultAppName+"/v1.0/repositories/?limit=100&offset=99", link)
}

func TestCreateLinkPreservesPathAndQueryEncoding(t *testing.T) {
	for _, path := range []string{
		api.FullRootPath() + "/repositories/",
		api.MajorRootPath() + "/tasks/",
		"/api/lightwell/v0.1/repositories/repo/python_packages/fixture/3.0.1%2Brhlw.2/files",
		"/api/lightwell/v0.1/repositories/repo/python_packages/fixture/3.0.1%252Brhlw.2/files",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path+"?search=C%2B%2B+%26+data%2Fscience&sort_by=name%3Adesc&arch=x86_64&limit=1&offset=0", nil)
			c := echo.New().NewContext(req, httptest.NewRecorder())
			link, err := url.Parse(createLink(c, 5, 10))
			require.NoError(t, err)
			assert.Empty(t, link.Host)
			assert.Equal(t, req.URL.EscapedPath(), link.EscapedPath())
			assert.Equal(t, "C++ & data/science", link.Query().Get("search"))
			assert.Equal(t, "name:desc", link.Query().Get("sort_by"))
			assert.Equal(t, "x86_64", link.Query().Get("arch"))
			assert.Equal(t, "5", link.Query().Get("limit"))
			assert.Equal(t, "10", link.Query().Get("offset"))
		})
	}
}

func TestCollectionResponsePageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name          string
		limit, offset int
		count         int64
		last          string
		next, prev    string
	}{
		{name: "middle page", limit: 3, offset: 3, count: 10, last: "9", next: "6", prev: "0"},
		{name: "past final page", limit: 5, offset: 10, count: 10, last: "5", prev: "5"},
		{name: "empty collection", limit: 100, count: 0, last: "0"},
		{name: "unlimited shared links", limit: -1, count: 3, last: "4", next: "-1", prev: "1"},
		{name: "negative limit and offset", limit: -2, offset: -3, count: 3, last: "2", next: "-5"},
		{name: "next offset overflow", limit: 200, offset: math.MaxInt, count: 3, last: "0", prev: strconv.Itoa(math.MaxInt - 200)},
		{name: "next offset underflow", limit: -1, offset: math.MinInt, count: 3, last: "4"},
		{name: "minimum integers", limit: math.MinInt, offset: math.MinInt, count: 3, last: "0", prev: "0"},
		{name: "last offset overflow", limit: -1, count: math.MaxInt, last: strconv.Itoa(math.MaxInt), next: "-1", prev: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collection := api.RepositoryCollectionResponse{}
			SetCollectionResponseMetadataForPage(&collection, getTestContext("?sort_by=name:asc"), api.PaginationData{Limit: tc.limit, Offset: tc.offset}, tc.count)
			assert.Equal(t, api.ResponseMetadata{Count: tc.count, Limit: tc.limit, Offset: tc.offset}, collection.Meta)
			for _, link := range []struct{ value, offset string }{
				{collection.Links.First, "0"}, {collection.Links.Last, tc.last},
				{collection.Links.Next, tc.next}, {collection.Links.Prev, tc.prev},
			} {
				if link.offset == "" {
					assert.Empty(t, link.value)
					continue
				}
				parsed, err := url.Parse(link.value)
				require.NoError(t, err)
				assert.Equal(t, api.FullRootPath()+"/repositories/", parsed.Path)
				assert.Equal(t, link.offset, parsed.Query().Get("offset"))
				assert.Equal(t, strconv.Itoa(tc.limit), parsed.Query().Get("limit"))
				assert.Equal(t, "name:asc", parsed.Query().Get("sort_by"))
			}
		})
	}
}
