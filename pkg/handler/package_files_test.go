package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	filesTestUUID     = "550e8400-e29b-41d4-a716-446655440000"
	filesTestBasePath = "python/remediated"
	filesTestDomain   = "trusted-owner-domain"
	filesTestVersion  = "3.0.1+rhlw.2"
)

type pythonFilesHandlerTest struct {
	t       *testing.T
	c       echo.Context
	rr      *httptest.ResponseRecorder
	reg     *dao.MockDaoRegistry
	pulp    *pulp_client.MockPulpClient
	handler PackageHandler
	repo    api.RepositoryResponse
}

func newPythonFilesHandlerTest(t *testing.T, query string) *pythonFilesHandlerTest {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	path := "/api/lightwell/v0.1/repositories/" + filesTestUUID + "/python_packages/Lightwell__Fixture/3.0.1%2Brhlw.2/files"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(identity.WithIdentity(req.Context(), test_handler.MockIdentity))
	rr := httptest.NewRecorder()
	c := e.NewContext(req, rr)
	c.SetPath("/api/lightwell/v0.1/repositories/:uuid/python_packages/:name/:version/files")
	c.SetParamNames("uuid", "name", "version")
	c.SetParamValues(filesTestUUID, "Lightwell__Fixture", filesTestVersion)
	reg := dao.GetMockDaoRegistry(t)
	pulp := pulp_client.NewMockPulpClient(t)
	return &pythonFilesHandlerTest{
		t: t, c: c, rr: rr, reg: reg, pulp: pulp,
		handler: PackageHandler{DaoRegistry: *reg.ToDaoRegistry(), PulpClient: pulp},
		repo: api.RepositoryResponse{
			UUID: filesTestUUID, OrgID: config.LightwellOrg, Origin: config.OriginLightwell,
			ContentType: config.ContentTypePython, PublishedDistBasePath: filesTestBasePath,
			PublishedDistURL: "https://untrusted-url.invalid/wrong-domain/wrong-path/",
		},
	}
}

func (f *pythonFilesHandlerTest) expectRepository() {
	f.t.Helper()
	f.reg.RepositoryConfig.On("List", f.c.Request().Context(), test_handler.MockOrgId,
		api.PaginationData{Limit: 1}, api.FilterData{UUID: filesTestUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{f.repo}}, int64(1), nil).Once()
}

func (f *pythonFilesHandlerTest) expectInventory(files []zest.PythonPythonPackageContentResponse, err error) {
	f.t.Helper()
	f.reg.Domain.On("Fetch", f.c.Request().Context(), f.repo.OrgID).Return(filesTestDomain, nil).Once()
	scoped := pulp_client.NewMockPulpClient(f.t)
	f.pulp.On("WithDomain", filesTestDomain).Return(scoped).Once()
	scoped.On("ListPythonPackageFiles", f.c.Request().Context(), filesTestBasePath, "lightwell-fixture", f.c.Param("version")).Return(files, err).Once()
}

func (f *pythonFilesHandlerTest) serve() *httptest.ResponseRecorder {
	f.t.Helper()
	if err := f.handler.ListPythonPackageFiles(f.c); err != nil {
		f.c.Error(err)
	}
	return f.rr
}

func (f *pythonFilesHandlerTest) response() api.PythonPackageFileCollectionResponse {
	f.t.Helper()
	rr := f.serve()
	require.Equal(f.t, http.StatusOK, rr.Code, rr.Body.String())
	var response api.PythonPackageFileCollectionResponse
	require.NoError(f.t, json.Unmarshal(rr.Body.Bytes(), &response))
	return response
}

func pythonFilesTestInventory() []zest.PythonPythonPackageContentResponse {
	files := make([]zest.PythonPythonPackageContentResponse, 3)
	for i, name := range []string{"A.whl", "b.tar.gz", "c.whl"} {
		files[i] = zest.PythonPythonPackageContentResponse{
			Filename: zest.PtrString(name), PulpHref: zest.PtrString("/internal/content/" + name),
			Artifact: zest.PtrString("/internal/artifacts/" + name),
		}
	}
	return files
}

func TestPythonPackageFilesMappingAndNullableFields(t *testing.T) {
	f := newPythonFilesHandlerTest(t, "")
	f.expectRepository()
	digest := strings.Repeat("ab", 32)
	files := []zest.PythonPythonPackageContentResponse{
		{Filename: zest.PtrString("a.whl"), Packagetype: zest.PtrString("bdist_wheel"), Size: zest.PtrInt64(0), Sha256: &digest},
		{Filename: zest.PtrString("b.tar.gz"), Packagetype: zest.PtrString("sdist"), Size: zest.PtrInt64(12)},
		{Filename: zest.PtrString("c.whl"), Packagetype: zest.PtrString("future_type")},
		{Filename: zest.PtrString("d.whl")},
		{Filename: zest.PtrString("e.tar.gz")},
		{Filename: zest.PtrString("f.unfamiliar"), PulpHref: zest.PtrString("/private/content"), Artifact: zest.PtrString("/private/artifact")},
	}
	f.expectInventory(files, nil)
	rr := f.serve()
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var response struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.Len(t, response.Data, 6)
	for _, item := range response.Data {
		assert.Len(t, item, 5)
		for _, key := range []string{"filename", "category", "packagetype", "size", "sha256"} {
			assert.Contains(t, item, key)
		}
	}
	assert.Equal(t, "package", response.Data[0]["category"])
	assert.Equal(t, float64(0), response.Data[0]["size"])
	assert.Equal(t, digest, response.Data[0]["sha256"])
	assert.Equal(t, "package", response.Data[1]["category"])
	assert.Equal(t, "unknown", response.Data[2]["category"])
	assert.Equal(t, "future_type", response.Data[2]["packagetype"])
	assert.Equal(t, "bdist_wheel", response.Data[3]["packagetype"])
	assert.Equal(t, "sdist", response.Data[4]["packagetype"])
	assert.Equal(t, "unknown", response.Data[5]["category"])
	assert.Nil(t, response.Data[5]["packagetype"])
	assert.Nil(t, response.Data[5]["size"])
	assert.Nil(t, response.Data[5]["sha256"])
	assert.NotContains(t, rr.Body.String(), "/private/")
	assert.NotContains(t, rr.Body.String(), "last_modified")
	assert.NotContains(t, rr.Body.String(), "signature_type")
}

func TestPythonPackageFilesValidationStopsBeforeRepositoryLookup(t *testing.T) {
	for _, tc := range []struct{ name, uuid, packageName, version, query string }{
		{name: "UUID", uuid: "not-a-uuid", packageName: "fixture", version: filesTestVersion},
		{name: "empty name", uuid: filesTestUUID, version: filesTestVersion},
		{name: "name begins separator", uuid: filesTestUUID, packageName: "_fixture", version: filesTestVersion},
		{name: "name ends separator", uuid: filesTestUUID, packageName: "fixture.", version: filesTestVersion},
		{name: "name spaces", uuid: filesTestUUID, packageName: "bad name", version: filesTestVersion},
		{name: "name slash", uuid: filesTestUUID, packageName: "bad/name", version: filesTestVersion},
		{name: "name non-ascii", uuid: filesTestUUID, packageName: "fíxture", version: filesTestVersion},
		{name: "unicode must not normalize into ASCII", uuid: filesTestUUID, packageName: "K", version: filesTestVersion},
		{name: "empty version", uuid: filesTestUUID, packageName: "fixture"},
		{name: "sort field", uuid: filesTestUUID, packageName: "fixture", version: filesTestVersion, query: "sort_by=size"},
		{name: "sort direction", uuid: filesTestUUID, packageName: "fixture", version: filesTestVersion, query: "sort_by=filename:bad"},
		{name: "sort empty direction", uuid: filesTestUUID, packageName: "fixture", version: filesTestVersion, query: "sort_by=filename:"},
		{name: "multiple sorts", uuid: filesTestUUID, packageName: "fixture", version: filesTestVersion, query: "sort_by=filename,size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, tc.query)
			f.c.SetParamValues(tc.uuid, tc.packageName, tc.version)
			rr := f.serve()
			assert.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
}

func TestPythonPackageFilesRepositoryGuardsAndMappings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		change     func(*pythonFilesHandlerTest)
		status     int
		domain     bool
		domainName string
		domainErr  error
	}{
		{name: "non-Lightwell origin", change: func(f *pythonFilesHandlerTest) { f.repo.Origin = config.OriginExternal }, status: 404},
		{name: "wrong content type", change: func(f *pythonFilesHandlerTest) { f.repo.ContentType = config.ContentTypeMaven }, status: 400},
		{name: "missing base path", change: func(f *pythonFilesHandlerTest) { f.repo.PublishedDistBasePath = "" }, status: 500},
		{name: "missing owner", change: func(f *pythonFilesHandlerTest) { f.repo.OrgID = "" }, status: 500},
		{name: "missing domain", change: func(f *pythonFilesHandlerTest) {}, status: 500, domain: true},
		{name: "domain failure", change: func(f *pythonFilesHandlerTest) {}, status: 500, domain: true, domainErr: errors.New("secret https://admin:password@internal.invalid/raw-body")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, "")
			tc.change(f)
			f.expectRepository()
			if tc.domain {
				f.reg.Domain.On("Fetch", f.c.Request().Context(), f.repo.OrgID).Return(tc.domainName, tc.domainErr).Once()
			}
			rr := f.serve()
			assert.Equal(t, tc.status, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "secret")
			assert.NotContains(t, rr.Body.String(), "internal.invalid")
			assert.NotContains(t, rr.Body.String(), "password")
		})
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "hidden or unentitled repository", status: 404},
		{name: "repository lookup failure", err: errors.New("secret https://admin:password@internal.invalid/raw-body"), status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, "")
			f.reg.RepositoryConfig.On("List", f.c.Request().Context(), test_handler.MockOrgId,
				api.PaginationData{Limit: 1}, api.FilterData{UUID: filesTestUUID}).Return(api.RepositoryCollectionResponse{}, int64(0), tc.err).Once()
			rr := f.serve()
			assert.Equal(t, tc.status, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "secret")
			assert.NotContains(t, rr.Body.String(), "internal.invalid")
			assert.NotContains(t, rr.Body.String(), "password")
		})
	}
}

func TestPythonPackageFilesInventoryFailureIsSanitized(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, errors.New("late page: https://admin:password@internal.invalid/ raw-body secret")} {
		t.Run(err.Error(), func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, "")
			f.expectRepository()
			f.expectInventory(pythonFilesTestInventory(), err)
			rr := f.serve()
			assert.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			for _, private := range []string{"admin", "password", "internal.invalid", "raw-body", "secret", "A.whl", "deadline exceeded"} {
				assert.NotContains(t, rr.Body.String(), private)
			}
			var response map[string]any
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
			assert.Contains(t, response, "errors")
			assert.NotContains(t, response, "data")
		})
	}
}

func TestPythonPackageFilesExactVersionAndOwnerDomain(t *testing.T) {
	for _, owner := range []string{config.LightwellOrg, config.LightwellDemoOrg} {
		for _, version := range []string{"3.0.1+rhlw.1", "3.0.1+rhlw.2", "3.0.1", " 3.0.1+rhlw.2 "} {
			t.Run(owner+"/"+version, func(t *testing.T) {
				f := newPythonFilesHandlerTest(t, "")
				f.repo.OrgID = owner
				f.c.SetParamValues(filesTestUUID, "Lightwell__Fixture", version)
				f.expectRepository()
				f.expectInventory(pythonFilesTestInventory(), nil)
				assert.Len(t, f.response().Data, 3)
			})
		}
	}
}

func TestPythonPackageFilesEmptyInventoryIsNotBeyondEndPage(t *testing.T) {
	for _, files := range [][]zest.PythonPythonPackageContentResponse{nil, {}} {
		f := newPythonFilesHandlerTest(t, "offset=1000")
		f.expectRepository()
		f.expectInventory(files, nil)
		rr := f.serve()
		assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	}
	for _, query := range []string{"offset=3", "offset=4&limit=-1", "offset=" + strconv.Itoa(math.MaxInt) + "&limit=-2"} {
		t.Run(query, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, query)
			f.expectRepository()
			f.expectInventory(pythonFilesTestInventory(), nil)
			response := f.response()
			assert.NotNil(t, response.Data)
			assert.Empty(t, response.Data)
			assert.Equal(t, int64(3), response.Meta.Count)
		})
	}
}

func TestPythonPackageFilesPublicPaginationAndSorting(t *testing.T) {
	for _, tc := range []struct {
		query         string
		limit, offset int
		names         []string
	}{
		{query: "", limit: 100, names: []string{"A.whl", "b.tar.gz", "c.whl"}},
		{query: "limit=0", limit: 100, names: []string{"A.whl", "b.tar.gz", "c.whl"}},
		{query: "limit=9999", limit: 200, names: []string{"A.whl", "b.tar.gz", "c.whl"}},
		{query: "limit=1&offset=1", limit: 1, offset: 1, names: []string{"b.tar.gz"}},
		{query: "limit=-1&offset=1", limit: -1, offset: 1, names: []string{"b.tar.gz", "c.whl"}},
		{query: "limit=-2", limit: -2, names: []string{"A.whl", "b.tar.gz", "c.whl"}},
		{query: "offset=-3&limit=2", limit: 2, offset: -3, names: []string{"A.whl", "b.tar.gz"}},
		{query: "sort_by=filename&limit=2", limit: 2, names: []string{"A.whl", "b.tar.gz"}},
		{query: "sort_by=filename:asc&limit=2", limit: 2, names: []string{"A.whl", "b.tar.gz"}},
		{query: "sort_by=filename:desc&limit=2", limit: 2, names: []string{"c.whl", "b.tar.gz"}},
		{query: "limit=invalid&offset=2&sort_by=filename:desc", limit: 100, names: []string{"A.whl", "b.tar.gz", "c.whl"}},
		{query: "limit=2&offset=invalid&sort_by=filename:desc", limit: 2, names: []string{"A.whl", "b.tar.gz"}},
		{query: "limit=" + strconv.Itoa(math.MaxInt) + "&offset=1", limit: 200, offset: 1, names: []string{"b.tar.gz", "c.whl"}},
		{query: "limit=" + strconv.Itoa(math.MinInt) + "&offset=" + strconv.Itoa(math.MinInt), limit: math.MinInt, offset: math.MinInt, names: []string{"A.whl", "b.tar.gz", "c.whl"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, tc.query)
			f.expectRepository()
			f.expectInventory(pythonFilesTestInventory(), nil)
			response := f.response()
			var names []string
			for _, item := range response.Data {
				names = append(names, item.Filename)
			}
			assert.Equal(t, tc.names, names)
			assert.Equal(t, api.ResponseMetadata{Count: 3, Limit: tc.limit, Offset: tc.offset}, response.Meta)
		})
	}
}

func TestPythonPackageFilesPaginationLinksPreserveCoordinatesAndSharedSemantics(t *testing.T) {
	for _, tc := range []struct {
		query      string
		next, prev string
	}{
		{query: "limit=1&offset=1&sort_by=filename:desc", next: "2", prev: "0"},
		{query: "limit=-2&offset=-3&sort_by=filename:asc", next: "-5"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, tc.query)
			f.expectRepository()
			f.expectInventory(pythonFilesTestInventory(), nil)
			response := f.response()
			for _, pair := range [][2]string{{response.Links.First, "0"}, {response.Links.Last, "2"}, {response.Links.Next, tc.next}, {response.Links.Prev, tc.prev}} {
				if pair[1] == "" {
					assert.Empty(t, pair[0])
					continue
				}
				link, err := url.Parse(pair[0])
				require.NoError(t, err)
				assert.Equal(t, f.c.Request().URL.Path, link.Path)
				assert.Contains(t, link.EscapedPath(), "3.0.1%2Brhlw.2")
				assert.Equal(t, "", link.Host)
				assert.Equal(t, pair[1], link.Query().Get("offset"))
				assert.Equal(t, f.c.QueryParam("sort_by"), link.Query().Get("sort_by"))
				assert.Equal(t, f.c.QueryParam("limit"), link.Query().Get("limit"))
			}
		})
	}
}

func TestPythonPackageFilesLargeOffsetLinksDoNotWrap(t *testing.T) {
	f := newPythonFilesHandlerTest(t, fmt.Sprintf("limit=200&offset=%d", math.MaxInt))
	f.expectRepository()
	f.expectInventory(pythonFilesTestInventory(), nil)
	response := f.response()
	assert.Empty(t, response.Data)
	assert.Empty(t, response.Links.Next, "overflow must not produce a next page for a beyond-end offset")
	prev, err := url.Parse(response.Links.Prev)
	require.NoError(t, err)
	assert.Equal(t, strconv.Itoa(math.MaxInt-200), prev.Query().Get("offset"))
}
