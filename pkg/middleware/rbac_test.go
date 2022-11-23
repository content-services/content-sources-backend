package middleware

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
)

func TestFromHttpVerbToRbacVerb(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    string
		Expected client.RbacVerb
	}

	testCases := []TestCase{
		{
			Name:     "empty method",
			Given:    "",
			Expected: client.VerbUndefined,
		},
		{
			Name:     "non existing method",
			Given:    "ANYOTHERTHING",
			Expected: client.VerbUndefined,
		},
		{
			Name:     "GET",
			Given:    echo.GET,
			Expected: client.VerbRead,
		},
		{
			Name:     "POST",
			Given:    echo.POST,
			Expected: client.VerbWrite,
		},
		{
			Name:     "PUT",
			Given:    echo.PUT,
			Expected: client.VerbWrite,
		},
		{
			Name:     "PATCH",
			Given:    echo.PATCH,
			Expected: client.VerbWrite,
		},
		{
			Name:     "DELETE",
			Given:    echo.DELETE,
			Expected: client.VerbWrite,
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result := fromHttpVerbToRbacVerb(testCase.Given)
		assert.Equal(t, testCase.Expected, result)
	}
}

func TestFromPathToResource(t *testing.T) {

}

func mockXRhUserIdentity(org_id string, accNumber string) string {

}

func server(req echo.Request) echo.Response {
	var (
		xrhid string = mockXRhUserIdentity("12345", "12345")
	)
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
	m := WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipLiveness)

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
	e.GET(URLPrefix+"/v1/ping", h)
	e.GET(URLPrefix+"/v1.0/ping", h)
	e.GET(URLPrefix+"/v1/repository_parameters/", h)

	// A Success request to /ping family path
	listSuccessPaths = []string{
		"/ping",
		URLPrefix + "/v1/ping",
		URLPrefix + "/v1.0/ping",
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
	req = httptest.NewRequest(http.MethodGet, URLPrefix+"/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderFailure)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Bad Request: x-rh-identity header is missing type\n", rec.Body.String())

	// A Success request with a right header
	req = httptest.NewRequest(http.MethodGet, URLPrefix+"/v1/repository_parameters/", nil)
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
	listSuccessPaths = []string{"/ping", URLPrefix + "/v1/ping", URLPrefix + "/v1.0/ping"}
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
