package handler

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

const (
	lightwellInternalValidateHeader = "X-Rh-Cs-Internal-Token"
	// LightwellInternalValidatePath is cluster-local only (not under /api/content-sources/).
	// Public console.redhat.com / 3scale only routes /api/content-sources/*, so this path is not internet-facing.
	LightwellInternalValidatePath = "/internal/lightwell/tokens/validate"
)

type LightwellTokenHandler struct {
	DaoRegistry          dao.DaoRegistry
	FeatureServiceClient feature_service_client.FeatureServiceClient
}

func checkLightwellAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckLightwellAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func checkOrgAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := requireOrgAdmin(c); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterLightwellTokenRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, fsClient feature_service_client.FeatureServiceClient) {
	h := LightwellTokenHandler{
		DaoRegistry:          *daoReg,
		FeatureServiceClient: fsClient,
	}
	addRepoRoute(engine, http.MethodPost, "/tokens/", h.createToken, rbac.RbacVerbWrite, checkLightwellAccessible, checkOrgAdmin)
	addRepoRoute(engine, http.MethodGet, "/tokens/", h.listTokens, rbac.RbacVerbRead, checkLightwellAccessible, checkOrgAdmin)
	addRepoRoute(engine, http.MethodDelete, "/tokens/:uuid", h.revokeToken, rbac.RbacVerbWrite, checkLightwellAccessible, checkOrgAdmin)
}

// RegisterLightwellInternalRoutes mounts service-to-service routes on the root Echo instance
// (outside /api/content-sources/), so they are not published via console.redhat.com.
func RegisterLightwellInternalRoutes(e *echo.Echo, daoReg *dao.DaoRegistry, fsClient feature_service_client.FeatureServiceClient) {
	h := LightwellTokenHandler{
		DaoRegistry:          *daoReg,
		FeatureServiceClient: fsClient,
	}
	e.Add(http.MethodPost, LightwellInternalValidatePath, h.validateToken)
}

// CreateLightwellToken godoc
// @Summary      Create a Lightwell access token
// @ID           createLightwellToken
// @Description  Create a personal Lightwell access token. Only org admins may create tokens. The plaintext token is returned once. access_level must be validated (validated+remediated) or remediated (remediated only).
// @Tags         lightwell_tokens
// @Accept       json
// @Produce      json
// @Param        body body api.LightwellTokenCreateRequest true "request body"
// @Success      201 {object} api.LightwellTokenResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tokens/ [post]
func (h *LightwellTokenHandler) createToken(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	callerUserID, err := getUserID(c)
	if err != nil {
		return err
	}

	var req api.LightwellTokenCreateRequest
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	userID := callerUserID
	if req.UserID != nil && *req.UserID != "" {
		userID = *req.UserID
	}

	if err := h.requireLightwellNetwork(c, orgID); err != nil {
		return err
	}

	created, err := h.DaoRegistry.LightwellToken.Create(c.Request().Context(), orgID, userID, req.Name, req.AccessLevel, req.ExpiresAt)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating Lightwell token", err.Error())
	}
	return c.JSON(http.StatusCreated, created)
}

// ListLightwellTokens godoc
// @Summary      List Lightwell access tokens
// @ID           listLightwellTokens
// @Description  List Lightwell access tokens for the caller's organization. Only org admins may list tokens.
// @Tags         lightwell_tokens
// @Accept       json
// @Produce      json
// @Success      200 {array} api.LightwellTokenResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tokens/ [get]
func (h *LightwellTokenHandler) listTokens(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	tokens, err := h.DaoRegistry.LightwellToken.ListByOrg(c.Request().Context(), orgID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing Lightwell tokens", err.Error())
	}
	return c.JSON(http.StatusOK, tokens)
}

// RevokeLightwellToken godoc
// @Summary      Revoke a Lightwell access token
// @ID           revokeLightwellToken
// @Description  Revoke a Lightwell access token by UUID. Only org admins may revoke tokens.
// @Tags         lightwell_tokens
// @Param        uuid path string true "Token UUID"
// @Success      204 "No Content"
// @Failure      401 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tokens/{uuid} [delete]
func (h *LightwellTokenHandler) revokeToken(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")
	if err := h.DaoRegistry.LightwellToken.Revoke(c.Request().Context(), orgID, uuid); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error revoking Lightwell token", err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// validateToken is a cluster-local Pulp callback. It is intentionally not OpenAPI-documented
// under the public Content Sources API (not under /api/content-sources/).
func (h *LightwellTokenHandler) validateToken(c echo.Context) error {
	if err := requireInternalValidateSecret(c); err != nil {
		return err
	}

	var req api.LightwellTokenValidateRequest
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	validated, err := h.DaoRegistry.LightwellToken.Validate(c.Request().Context(), req.Token)
	if err != nil {
		return ce.NewErrorResponse(http.StatusUnauthorized, "Invalid Lightwell token", err.Error())
	}

	if err := h.requireLightwellNetwork(c, validated.OrgID); err != nil {
		return err
	}

	if req.Path != "" {
		securityLevel := config.SecurityLevelFromContentPath(req.Path)
		if securityLevel == "" {
			return ce.NewErrorResponse(http.StatusForbidden, "Insufficient access level",
				"could not determine security level from path")
		}
		if !config.LightwellTokenAllows(validated.AccessLevel, securityLevel) {
			return ce.NewErrorResponse(http.StatusForbidden, "Insufficient access level",
				fmt.Sprintf("token access_level %q cannot access security_level %q", validated.AccessLevel, securityLevel))
		}
	}

	return c.JSON(http.StatusOK, validated)
}

func (h *LightwellTokenHandler) requireLightwellNetwork(c echo.Context, orgID string) error {
	features, err := h.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error getting entitled features for Lightwell token check")
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error checking entitlements", err.Error())
	}
	if !slices.Contains(features, config.LightwellNetworkFeature) {
		return ce.NewErrorResponse(http.StatusForbidden, "Missing entitlement", "Account does not have the lightwell-network entitlement")
	}
	return nil
}

// CheckLightwellAccessible reports whether the Lightwell feature is enabled for the caller.
func CheckLightwellAccessible(ctx context.Context) error {
	if !config.Get().Features.Lightwell.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Lightwell feature is disabled.", "")
	}
	if config.FeatureAccessible(ctx, config.Get().Features.Lightwell) {
		return nil
	}
	return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage Lightwell tokens",
		"Neither the user nor the account is allowed.")
}

func requireOrgAdmin(c echo.Context) error {
	return requireOrgAdminFor(c, "manage Lightwell tokens")
}

func requireOrgAdminFor(c echo.Context, action string) error {
	id := identity.Get(c.Request().Context())
	if id.Identity.Type != "User" || id.Identity.User == nil {
		return ce.NewErrorResponse(http.StatusForbidden, "Org admin required", "Only organization administrators can "+action)
	}
	if !id.Identity.User.OrgAdmin {
		return ce.NewErrorResponse(http.StatusForbidden, "Org admin required", "Only organization administrators can "+action)
	}
	return nil
}

func requireInternalValidateSecret(c echo.Context) error {
	expected := config.Get().Options.LightwellValidateSecret
	if expected == "" {
		return ce.NewErrorResponse(http.StatusUnauthorized, "Unauthorized", "internal validate secret is not configured")
	}
	provided := c.Request().Header.Get(lightwellInternalValidateHeader)
	if provided == "" || provided != expected {
		return ce.NewErrorResponse(http.StatusUnauthorized, "Unauthorized", "invalid or missing internal token")
	}
	return nil
}
