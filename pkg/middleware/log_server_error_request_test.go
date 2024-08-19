package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

const TestBody = `{"limit": 100, "search": "random", "uuids": ["e88efd75-2b29-4b59-8867-bb60435b3742"]}`
const LargeBody = `{"limit": 100, "search": "Lorem ipsum dolor sit amet, consectetuer adipiscing elit. Integer pellentesque quam vel velit. Vivamus porttitor turpis ac leo. Ut tempus purus at lorem. Nullam justo enim, consectetuer nec, ullamcorper ac, vestibulum in, elit. Nulla pulvinar eleifend sem. Proin mattis lacinia justo. Integer malesuada. Etiam sapien elit, consequat eget, tristique non, venenatis quis, ante. Etiam dui sem, fermentum vitae, sagittis id, malesuada in, quam. Integer lacinia. Proin pede metus, vulputate nec, fermentum fringilla, vehicula vitae, justo. Curabitur bibendum justo non orci. Ut enim ad minima veniam, quis nostrum exercitationem ullam corporis suscipit laboriosam, nisi ut aliquid ex ea commodi consequatur? Etiam sapien elit, consequat eget, tristique non, venenatis quis, ante. Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas. Maecenas sollicitudin. Quisque porta. Nullam sapien sem, ornare ac, nonummy non, lobortis a enim. Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas. Maecenas sollicitudin. Quisque porta. Nullam sapien sem, ornare ac, nonummy non, lobortis a enim.", "uuids": ["e88efd75-2b29-4b59-8867-bb60435b3742"]}`

func TestReadBodyStoreAndRestore(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(TestBody))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := LogServerErrorRequest(func(c echo.Context) error {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": fmt.Sprintf("%d", http.StatusInternalServerError),
			"title":  "testing error",
		})
	})

	_ = h(c)

	var reqBody []byte
	if c.Request().Body != nil {
		reqBody, _ = io.ReadAll(c.Request().Body)
	}

	storedBody := c.Get(BodyStoreKey)
	storedBodyBytes, ok := storedBody.([]byte)
	if !ok {
		assert.Fail(t, "Failed type assertion of stored body.")
	}

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, TestBody, string(storedBodyBytes))
	assert.Equal(t, TestBody, string(reqBody))
}

func TestLargeBodyCutoff(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(LargeBody))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := LogServerErrorRequest(func(c echo.Context) error {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": fmt.Sprintf("%d", http.StatusInternalServerError),
			"title":  "testing error",
		})
	})

	_ = h(c)

	var reqBody []byte
	if c.Request().Body != nil {
		reqBody, _ = io.ReadAll(c.Request().Body)
	}

	storedBody := c.Get(BodyStoreKey)
	storedBodyBytes, ok := storedBody.([]byte)
	if !ok {
		assert.Fail(t, "Failed type assertion of stored body.")
	}

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, LargeBody[:1000], string(storedBodyBytes))
	assert.Equal(t, LargeBody, string(reqBody))
}

func TestRequestBodyLogging(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(TestBody))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(BodyStoreKey, []byte(TestBody))
	buf := new(bytes.Buffer)
	c.Logger().SetOutput(buf)
	h := LogServerErrorRequest(func(c echo.Context) error {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Test Error", "5xx testing error")
	})

	_ = h(c)

	log := make(map[string]string)
	_ = json.Unmarshal(buf.Bytes(), &log)

	assert.True(t, bytes.Contains(buf.Bytes(), []byte("\"message\":\"Request body: {\\\"limit\\\": 100, \\\"search\\\": \\\"random\\\", \\\"uuids\\\": [\\\"e88efd75-2b29-4b59-8867-bb60435b3742\\\"]}\"}")))
	assert.Equal(t, log["message"], fmt.Sprintf("Request body: %s", TestBody))
	assert.Equal(t, log["level"], "ERROR")
}

func TestMethodWithoutBody(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	buf := new(bytes.Buffer)
	c.Logger().SetOutput(buf)
	h := LogServerErrorRequest(func(c echo.Context) error {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Test Error", "5xx testing error")
	})

	_ = h(c)

	log := make(map[string]string)
	_ = json.Unmarshal(buf.Bytes(), &log)

	assert.Equal(t, []byte(nil), buf.Bytes())
}
