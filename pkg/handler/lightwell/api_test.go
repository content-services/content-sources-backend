package lightwell

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRouter() *echo.Echo {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	RegisterRoutes(context.Background(), router)
	return router
}

func TestOpenapi(t *testing.T) {
	req, _ := http.NewRequest("GET", LightwellAPIPath+"/openapi.json", nil)

	router := setupTestRouter()
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	code := rr.Code
	body := rr.Body.Bytes()

	assert.Equal(t, http.StatusOK, code)

	js := json.RawMessage{}
	err := json.Unmarshal(body, &js)
	require.NoError(t, err)
}

func TestLightwellAPIPath(t *testing.T) {
	assert.Equal(t, "/api/lightwell/v0.1", LightwellAPIPath)
}
