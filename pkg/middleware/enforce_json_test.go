package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func serveRouter(method string, contentType string, path string, includeBody bool) (int, []byte, error) {
	router := echo.New()
	router.Use(EnforceJSONContentType)
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler

	router.Add(method, path, func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"Status": "OK"})
	})

	var req *http.Request
	if includeBody {
		req = httptest.NewRequest(method, path, strings.NewReader("body"))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func TestEnforceJSONSkipper(t *testing.T) {
	router := echo.New()
	path := "/"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("body"))
	ctx := router.NewContext(req, nil)
	assert.False(t, enforceJSONContentTypeSkipper(ctx))

	// Skips check if no body
	noBodyReq := httptest.NewRequest(http.MethodGet, path, nil)
	noBodyCtx := router.NewContext(noBodyReq, nil)
	assert.True(t, enforceJSONContentTypeSkipper(noBodyCtx))
}

func TestJSONContentType(t *testing.T) {
	path := "/"
	status, _, err := serveRouter(http.MethodPatch, "application/json", path, true)
	assert.Equal(t, http.StatusOK, status)
	assert.NoError(t, err)

	withParameterStatus, _, withParameterErr := serveRouter(http.MethodPut, "application/json; parameter=value", path, true)
	assert.Equal(t, http.StatusOK, withParameterStatus)
	assert.NoError(t, withParameterErr)
}

func TestInvalidContentType(t *testing.T) {
	testCases := []struct {
		method       string
		contentType  string
		errorMessage string
	}{
		{http.MethodPost, "text/html", "Incorrect content type"},
		{http.MethodPatch, "not a valid content type", "Error parsing content type"},
		{http.MethodPut, "", "Error parsing content type"},
	}

	for _, testCase := range testCases {
		status, body, err := serveRouter(testCase.method, testCase.contentType, "/", true)
		assert.Equal(t, http.StatusUnsupportedMediaType, status)
		assert.NoError(t, err)
		assert.Contains(t, string(body), testCase.errorMessage)
	}
}

func TestSkippedRoutes(t *testing.T) {
	status, _, err := serveRouter(http.MethodPost, "", "/", false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
}
