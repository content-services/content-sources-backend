package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

const LightwellBearerAuthContextKey = "lightwell_bearer_auth"
const LightwellBearerAccessLevelContextKey = "lightwell_bearer_access_level"

// Must match dao.lightwellTokenPrefix so Console SSO Bearer JWTs are not treated as Lightwell tokens.
const lightwellBearerTokenPrefix = "lw_"

// LightwellBearerAuth validates Authorization: Bearer Lightwell tokens, re-checks
// lightwell-network entitlement, and injects a synthetic identity for downstream handlers.
// Non-Lightwell Bearer values (e.g. Console SSO JWTs) are ignored so EnforceIdentity can run.
// Token management routes (/tokens/) reject Bearer auth and require Console identity + org admin.
func LightwellBearerAuth(daoReg *dao.DaoRegistry, fsClient feature_service_client.FeatureServiceClient) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw, ok := bearerToken(c.Request().Header.Get("Authorization"))
			if !ok {
				return next(c)
			}
			// Console/proxy often sends Authorization: Bearer <SSO JWT> alongside x-rh-identity.
			// Only Lightwell plaintext tokens use the lw_ prefix; anything else falls through.
			if !strings.HasPrefix(raw, lightwellBearerTokenPrefix) {
				return next(c)
			}

			path := getPath(c)
			if isLightwellTokenManagementPath(path) {
				err := ce.NewErrorResponse(http.StatusUnauthorized, "Unauthorized", "Bearer tokens cannot manage Lightwell tokens; use Console identity")
				c.Error(err)
				return nil
			}
			if isInternalLightwellValidatePath(path) {
				return next(c)
			}

			validated, err := daoReg.LightwellToken.Validate(c.Request().Context(), raw)
			if err != nil {
				errResp := ce.NewErrorResponse(http.StatusUnauthorized, "Unauthorized", "invalid Lightwell token")
				c.Error(errResp)
				return nil
			}

			features, err := fsClient.GetEntitledFeatures(c.Request().Context(), validated.OrgID)
			if err != nil {
				log.Error().Err(err).Msg("error getting entitled features for Lightwell bearer auth")
				errResp := ce.NewErrorResponse(http.StatusInternalServerError, "Error checking entitlements", err.Error())
				c.Error(errResp)
				return nil
			}
			if !slices.Contains(features, config.LightwellNetworkFeature) {
				errResp := ce.NewErrorResponse(http.StatusForbidden, "Missing entitlement", "Account does not have the lightwell-network entitlement")
				c.Error(errResp)
				return nil
			}

			xrhid := identity.XRHID{
				Identity: identity.Identity{
					Type: "User",
					Internal: identity.Internal{
						OrgID: validated.OrgID,
					},
					OrgID: validated.OrgID,
					User: &identity.User{
						UserID:   validated.UserID,
						Username: validated.UserID,
					},
				},
			}
			ctx := identity.WithIdentity(c.Request().Context(), xrhid)
			c.SetRequest(c.Request().WithContext(ctx))
			c.Set(LightwellBearerAuthContextKey, true)
			c.Set(LightwellBearerAccessLevelContextKey, validated.AccessLevel)

			return next(c)
		}
	}
}

func bearerToken(authorization string) (string, bool) {
	if authorization == "" {
		return "", false
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}

func isLightwellTokenManagementPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return path == "/tokens" || strings.HasPrefix(path, "/tokens/")
}

func isInternalLightwellValidatePath(path string) bool {
	return strings.TrimSuffix(path, "/") == "/internal/lightwell/tokens/validate"
}

func HasLightwellBearerAuth(c echo.Context) bool {
	v, ok := c.Get(LightwellBearerAuthContextKey).(bool)
	return ok && v
}

// LightwellBearerAccessLevel returns the access_level stashed by LightwellBearerAuth, or "".
func LightwellBearerAccessLevel(c echo.Context) string {
	v, _ := c.Get(LightwellBearerAccessLevelContextKey).(string)
	return v
}
