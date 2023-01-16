package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestMatchedRoute(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    string
		Expected string
	}
	testCases := []TestCase{
		{
			Name:     "no matching /",
			Given:    "/",
			Expected: "{\"message\":\"Not Found\"}\n",
		},
		{
			Name:     "matching /api/test",
			Given:    "/api/test",
			Expected: "/api/test",
		},
		{
			Name:     "matching /api/test/:id",
			Given:    "/api/test/12345",
			Expected: "/api/test/:id",
		},
		{
			Name:     "no matching /anything",
			Given:    "/api/anything",
			Expected: "{\"message\":\"Not Found\"}\n",
		},
		{
			Name:     "no matching /test/",
			Given:    "/api/test/",
			Expected: "{\"message\":\"Not Found\"}\n",
		},
	}
	h := func(c echo.Context) error {
		// The context.Path() is filled during serving the request,
		// so it is not enough create the context and call to matchedRoute
		match := MatchedRoute(c)
		return c.String(http.StatusOK, match)
	}
	for _, testCase := range testCases {
		t.Log(testCase.Name)
		e := echo.New()
		g := e.Group("/api")
		g.Add(http.MethodGet, "/test", h)
		g.Add(http.MethodGet, "/test/:id", h)

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testCase.Given, http.NoBody))
		assert.Equal(t, testCase.Expected, rec.Body.String())
	}
}
