package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

// extractOrgIds extracts org_id values from a generic response structure
// Handles both direct org_id fields and nested data arrays/objects
func extractOrgIds(response map[string]any) []string {
	var orgIds []string

	// Check for direct org_id field
	if orgId, exists := response["org_id"]; exists {
		if orgIdStr, ok := orgId.(string); ok && orgIdStr != "" {
			orgIds = append(orgIds, orgIdStr)
		}
	}

	// Check for data field containing array or object
	if data, exists := response["data"]; exists {
		switch dataValue := data.(type) {
		case []interface{}:
			// Handle array of objects (e.g., repository collections)
			for _, item := range dataValue {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if orgId, exists := itemMap["org_id"]; exists {
						if orgIdStr, ok := orgId.(string); ok && orgIdStr != "" {
							orgIds = append(orgIds, orgIdStr)
						}
					}
				}
			}
		case map[string]interface{}:
			// Handle single object in data field
			if orgId, exists := dataValue["org_id"]; exists {
				if orgIdStr, ok := orgId.(string); ok && orgIdStr != "" {
					orgIds = append(orgIds, orgIdStr)
				}
			}
		}
	}

	return orgIds
}

// EnforceConsistentOrgId middleware checks that the user's org ID from identity header
// matches the org_id in any returned data using a generic approach
func EnforceConsistentOrgId(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Skip middleware for routes that skip authentication
		if SkipMiddleware(c) {
			return next(c)
		}

		if SkipEnforceConsistentOrgId(c) {
			return next(c)
		}

		// Get user's org ID from identity header
		accountId, orgId := getAccountIdOrgId(c)
		if orgId == "" {
			return ce.NewErrorResponse(http.StatusBadRequest, "Missing org ID", "Organization ID is required")
		}

		// Create a recorder to capture the response
		rec := httptest.NewRecorder()
		originalResponse := c.Response()

		// Temporarily set the recorder as the response writer
		c.SetResponse(echo.NewResponse(rec, c.Echo()))

		// Call the next handler
		err := next(c)
		if err != nil {
			// Restore original response before returning error
			c.SetResponse(originalResponse)
			return err
		}

		// Parse the response body to check org_id consistency for any endpoint
		// Only validate for OK responses with non-empty bodies
		if rec.Code == http.StatusOK && rec.Body.Len() != 0 {
			var response map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				// Log JSON parsing error and skip validation
				log.Warn().Err(err).Msg("Failed to parse response JSON for org_id validation")
				// Continue to restore response and return
			} else {
				// Extract all org_id values from the response
				responseOrgIds := extractOrgIds(response)

				// Only check org_id consistency if the response actually contains org_id fields
				for _, responseOrgId := range responseOrgIds {
					if responseOrgId != orgId && responseOrgId != config.RedHatOrg && responseOrgId != config.CommunityOrg {
						log.Error().Str("user_org_id", orgId).Str("response_org_id", responseOrgId).Str("account_id", accountId).Msg("Org ID mismatch")
						// Restore original response before returning error
						c.SetResponse(originalResponse)
						return ce.NewErrorResponse(http.StatusInternalServerError, "Organization ID mismatch", "Response organization ID does not match user organization ID")
					}
				}
			}
		}

		// Restore original response and write the captured response
		c.SetResponse(originalResponse)

		// Copy headers from recorder to original response
		for key, values := range rec.Header() {
			for _, value := range values {
				c.Response().Header().Set(key, value)
			}
		}

		// Write status and body
		c.Response().WriteHeader(rec.Code)
		_, err = c.Response().Write(rec.Body.Bytes())
		return err
	}
}
func SkipEnforceConsistentOrgId(c echo.Context) bool {
	path := getPath(c)
	skipped := []string{
		"/admin/tasks/",
		"/admin/tasks/:task_uuid",
	}
	return utils.Contains(skipped, path)
}

// getAccountIdOrgId extracts account ID and org ID from the request context
func getAccountIdOrgId(c echo.Context) (string, string) {
	data := identity.GetIdentity(c.Request().Context())
	return data.Identity.AccountNumber, data.Identity.Internal.OrgID
}
