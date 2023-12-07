package middleware

import (
	"encoding/base64"
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
	xrhidentityHeaderBadOrgID := `{"identity":{"type":"Associate","account_number":"2093","internal":{"org_id":"-1"}}}`

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

	// A rejected request with org ID of -1
	req = httptest.NewRequest(http.MethodGet, urlPrefix+"/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderBadOrgID)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.Error(t, err)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
