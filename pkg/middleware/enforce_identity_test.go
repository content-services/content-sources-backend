package middleware

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
)

const urlPrefix = "/api/" + config.DefaultAppName

func TestSkipLivenessTrue(t *testing.T) {
	listRoutes := []string{
		"/ping",
		urlPrefix + "/v1.0/ping",
		urlPrefix + "/v1/ping",
	}
	e := echo.New()
	handler.RegisterPing(e)
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipAuth))

	for _, route := range listRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		res := httptest.NewRecorder()
		c := e.NewContext(req, res)

		result := SkipAuth(c)
		assert.True(t, result)
	}
}

func TestSkipLivenessFalse(t *testing.T) {
	listRoutes := []string{
		"/api/v1/repositories",
		"/api/v1/repositories/ping",
	}
	e := echo.New()
	handler.RegisterPing(e)
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipAuth))

	for _, route := range listRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		res := httptest.NewRecorder()
		c := e.NewContext(req, res)

		result := SkipAuth(c)
		assert.False(t, result)
	}
}

func TestWrapMiddlewareWithSkipper(t *testing.T) {
	var (
		req              *http.Request
		rec              *httptest.ResponseRecorder
		c                echo.Context
		h                func(c echo.Context) error
		err              error
		listSuccessPaths []string
	)
	e := echo.New()
	m := WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipAuth)

	IdentityHeader := "X-Rh-Identity"
	xrhidentityHeaderSuccess := `{"identity":{"type":"Associate","account_number":"2093","internal":{"org_id":"7066"}}}`
	xrhidentityHeaderFailure := `{"identity":{"account_number":"2093","internal":{"org_id":"7066"}}}`

	bodyResponse := "It Worded!"

	h = func(c echo.Context) error {
		body, err := []byte(bodyResponse), error(nil)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	}
	e.GET("/ping", h)
	e.GET(urlPrefix+"/v1/ping", h)
	e.GET(urlPrefix+"/v1.0/ping", h)
	e.GET(urlPrefix+"/v1/repository_parameters/", h)

	// A Success request to /ping family path
	listSuccessPaths = []string{
		"/ping",
		urlPrefix + "/v1/ping",
		urlPrefix + "/v1.0/ping",
	}
	for _, path := range listSuccessPaths {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderSuccess)))
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)

		err = m(h)(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, bodyResponse, rec.Body.String())
	}

	// A Failed request with failed header
	req = httptest.NewRequest(http.MethodGet, urlPrefix+"/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderFailure)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Bad Request: x-rh-identity header is missing type\n", rec.Body.String())

	// A Success request with a right header
	req = httptest.NewRequest(http.MethodGet, urlPrefix+"/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderSuccess)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, bodyResponse, rec.Body.String())
	encodedHeader := base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderSuccess))
	assert.Equal(t, encodedHeader, rec.Header().Get(IdentityHeader))

	// A Success request with failed header for /ping route
	// The middleware should skip for this route and call the
	// handler which fill the expected bodyResponse
	listSuccessPaths = []string{"/ping", urlPrefix + "/v1/ping", urlPrefix + "/v1.0/ping"}
	for _, path := range listSuccessPaths {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderFailure)))
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)

		err = m(h)(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, bodyResponse, rec.Body.String())
	}
}

func TestEnforceOrgId(t *testing.T) {
	e := echo.New()
	e.Use(EnforceOrgId)
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler

	// Test org ID of -1 returns an error response
	req := httptest.NewRequest(http.MethodGet, urlPrefix+"/v1/repository_parameters/", nil)

	var xrhid identity.XRHID

	xrhid.Identity.OrgID = "-1"
	xrhid.Identity.AccountNumber = "11111"
	xrhid.Identity.User.Username = "user"
	xrhid.Identity.Internal.OrgID = "-1"

	ctx := req.Context()
	ctx = context.WithValue(ctx, identity.Key, xrhid)
	req = req.WithContext(ctx)

	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	body, err := io.ReadAll(res.Body)

	assert.NoError(t, err)
	assert.Contains(t, string(body), "Invalid org ID")
	assert.Equal(t, http.StatusForbidden, res.Code)

	// Test valid org ID returns success
	xrhid.Identity.OrgID = "7066"
	xrhid.Identity.AccountNumber = "11111"
	xrhid.Identity.User.Username = "user"
	xrhid.Identity.Internal.OrgID = "7066"

	ctx = req.Context()
	ctx = context.WithValue(ctx, identity.Key, xrhid)
	req = req.WithContext(ctx)

	res = httptest.NewRecorder()
	e.Add(req.Method, req.URL.Path, handleItWorked)
	e.ServeHTTP(res, req)

	body, err = io.ReadAll(res.Body)

	assert.NoError(t, err)
	assert.Equal(t, "\"It worked\"\n", string(body))
	assert.Equal(t, http.StatusOK, res.Code)
}
