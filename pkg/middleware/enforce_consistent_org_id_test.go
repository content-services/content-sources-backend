package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
)

// encodeIdentity helper function to encode identity for testing
func encodeIdentity(xrhid identity.XRHID) string {
	jsonIdentity, _ := json.Marshal(xrhid)
	return base64.StdEncoding.EncodeToString(jsonIdentity)
}

func TestEnforceConsistentOrgId_Success(t *testing.T) {
	testOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity with matching org ID
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: testOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate repository list response with matching org ID using generic structure
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":       "test-uuid-123",
					"name":       "Test Repo",
					"url":        "http://example.com/repo",
					"org_id":     testOrgId, // Same as user's org ID
					"account_id": testAccountId,
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/repositories/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass without error
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEnforceConsistentOrgId_OrgIdMismatch(t *testing.T) {
	userOrgId := "user-org-456"
	repoOrgId := "repo-org-123"
	testAccountId := "test-account-789"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: userOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate repository list response with mismatched org ID using generic structure
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":       "test-uuid-123",
					"name":       "Test Repo",
					"url":        "http://example.com/repo",
					"org_id":     repoOrgId, // Different from user's org ID
					"account_id": testAccountId,
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/repositories/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should return 500 error due to org ID mismatch
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestEnforceConsistentOrgId_MissingOrgId(t *testing.T) {
	// Create mock identity without org ID
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: "test-account-123",
			Internal: identity.Internal{
				OrgID: "", // Missing org ID
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		response := map[string]any{
			"data": []map[string]any{},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/repositories/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should return 400 error due to missing org ID
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEnforceConsistentOrgId_NonRepositoryEndpoint(t *testing.T) {
	// Test that middleware doesn't interfere with non-repository endpoints
	testOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: testOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request for different endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/ping", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/ping", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass through without error
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGetAccountIdOrgId(t *testing.T) {
	testOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: testOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler to test the function
	testHandler := func(c echo.Context) error {
		// Test the getAccountIdOrgId function
		accountId, orgId := getAccountIdOrgId(c)

		// Assert within the handler
		assert.Equal(t, testAccountId, accountId)
		assert.Equal(t, testOrgId, orgId)

		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	// Add the route
	e.GET("/api/content-sources/v1/repositories/", testHandler)

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass through without error
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEnforceConsistentOrgId_GenericResponse_LowercaseOrgId(t *testing.T) {
	testOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity with matching org ID
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: testOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/tasks/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate task response with lowercase org_id format
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":   "task-uuid-123",
					"status": "completed",
					"org_id": testOrgId, // Using lowercase org_id format
					"type":   "introspection",
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/tasks/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass without error
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEnforceConsistentOrgId_GenericResponse_DirectOrgId(t *testing.T) {
	testOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity with matching org ID
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: testOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/features/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate direct org_id response (not nested in data)
		response := map[string]any{
			"org_id":       testOrgId, // Direct org_id field
			"feature_list": []string{"feature1", "feature2"},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/features/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass without error
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEnforceConsistentOrgId_GenericResponse_MismatchLowercaseOrgId(t *testing.T) {
	userOrgId := "user-org-456"
	responseOrgId := "response-org-123"
	testAccountId := "test-account-789"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: userOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/tasks/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate task response with mismatched lowercase org_id
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":   "task-uuid-123",
					"status": "completed",
					"org_id": responseOrgId, // Different from user's org ID
					"type":   "introspection",
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/tasks/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should return 500 error due to org ID mismatch
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestExtractOrgIds_VariousFormats(t *testing.T) {
	// Test direct org_id
	response1 := map[string]any{
		"org_id": "test-org-123",
		"other":  "data",
	}
	orgIds1 := extractOrgIds(response1)
	assert.Equal(t, []string{"test-org-123"}, orgIds1)

	// Test data array with org_id
	response2 := map[string]any{
		"data": []interface{}{
			map[string]interface{}{
				"uuid":   "item1",
				"org_id": "test-org-789",
			},
			map[string]interface{}{
				"uuid":   "item2",
				"org_id": "test-org-789",
			},
		},
	}
	orgIds2 := extractOrgIds(response2)
	assert.Equal(t, []string{"test-org-789", "test-org-789"}, orgIds2)

	// Test data object (single item)
	response3 := map[string]any{
		"data": map[string]interface{}{
			"uuid":   "single-item",
			"org_id": "test-org-single",
		},
	}
	orgIds3 := extractOrgIds(response3)
	assert.Equal(t, []string{"test-org-single"}, orgIds3)

	// Test empty response
	response4 := map[string]any{
		"other": "data",
	}
	orgIds4 := extractOrgIds(response4)
	assert.Empty(t, orgIds4)

	// Test empty data array
	response5 := map[string]any{
		"data": []interface{}{},
	}
	orgIds5 := extractOrgIds(response5)
	assert.Empty(t, orgIds5)
}

func TestEnforceConsistentOrgId_AllowRHELOrgId(t *testing.T) {
	userOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: userOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate repository response with RHEL org_id "-1"
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":       "rhel-repo-123",
					"name":       "RHEL Repository",
					"url":        "http://example.com/rhel-repo",
					"org_id":     "-1", // RHEL org_id should be allowed
					"account_id": testAccountId,
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/repositories/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass without error (RHEL org_id "-1" is allowed)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEnforceConsistentOrgId_AllowCommunityOrgId(t *testing.T) {
	userOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: userOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate repository response with community org_id "-2"
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":       "community-repo-123",
					"name":       "Community Repository",
					"url":        "http://example.com/community-repo",
					"org_id":     "-2", // Community org_id should be allowed
					"account_id": testAccountId,
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/repositories/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass without error (Community org_id "-2" is allowed)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEnforceConsistentOrgId_MixedOrgIds(t *testing.T) {
	userOrgId := "test-org-123"
	testAccountId := "test-account-456"

	// Create mock identity
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: testAccountId,
			Internal: identity.Internal{
				OrgID: userOrgId,
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1/repositories/", nil)
	rec := httptest.NewRecorder()

	// Setup echo with identity middleware
	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipMiddleware))

	// Set identity header
	encodedIdentity := encodeIdentity(xrhid)
	req.Header.Set("X-Rh-Identity", encodedIdentity)

	// Create handler chain with middleware
	testHandler := func(c echo.Context) error {
		// Simulate repository response with mixed org_ids (user's, RHEL, and community)
		response := map[string]any{
			"data": []map[string]any{
				{
					"uuid":       "user-repo-123",
					"name":       "User Repository",
					"url":        "http://example.com/user-repo",
					"org_id":     userOrgId, // User's org_id
					"account_id": testAccountId,
				},
				{
					"uuid":       "rhel-repo-123",
					"name":       "RHEL Repository",
					"url":        "http://example.com/rhel-repo",
					"org_id":     "-1", // RHEL org_id
					"account_id": testAccountId,
				},
				{
					"uuid":       "community-repo-123",
					"name":       "Community Repository",
					"url":        "http://example.com/community-repo",
					"org_id":     "-2", // Community org_id
					"account_id": testAccountId,
				},
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Add the route and middleware
	e.GET("/api/content-sources/v1/repositories/", EnforceConsistentOrgId(testHandler))

	// Execute
	e.ServeHTTP(rec, req)

	// Assert - should pass without error (all org_ids are allowed)
	assert.Equal(t, http.StatusOK, rec.Code)
}
