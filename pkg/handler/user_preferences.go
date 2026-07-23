package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

type UserPreferencesHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterUserPreferencesRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	h := UserPreferencesHandler{DaoRegistry: *daoReg}
	addRepoRoute(engine, http.MethodGet, "/user_preferences/", h.listUserPreferences, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPut, "/user_preferences/:label", h.setUserPreference, rbac.RbacVerbWrite)
}

// ListUserPreferences godoc
// @Summary      List user preferences
// @ID           listUserPreferences
// @Description  List preference labels and values for the authenticated user.
// @Tags         user_preferences
// @Accept       json
// @Produce      json
// @Success      200 {array} api.UserPreferenceResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /user_preferences/ [get]
func (h *UserPreferencesHandler) listUserPreferences(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	userID, err := getUserID(c)
	if err != nil {
		return err
	}

	prefs, err := h.DaoRegistry.UserPreference.List(c.Request().Context(), orgID, userID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing user preferences", err.Error())
	}
	return c.JSON(http.StatusOK, prefs)
}

// SetUserPreference godoc
// @Summary      Set a user preference
// @ID           setUserPreference
// @Description  Create or update a user preference for the given label. The request body is a JSON string value (for example `"true"`).
// @Tags         user_preferences
// @Accept       json
// @Produce      json
// @Param        label path string true "Preference label" Enums(lightwell-notification-enabled, lightwell-notification-minimum)
// @Param        body body string true "Preference value as a JSON string"
// @Success      200 {object} api.UserPreferenceResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /user_preferences/{label} [put]
func (h *UserPreferencesHandler) setUserPreference(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	userID, err := getUserID(c)
	if err != nil {
		return err
	}

	label := c.Param("label")
	value, err := readJSONStringBody(c)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding params", err.Error())
	}

	pref, err := h.DaoRegistry.UserPreference.Set(c.Request().Context(), orgID, userID, label, value)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error setting user preference", err.Error())
	}
	return c.JSON(http.StatusOK, pref)
}

func getUserID(c echo.Context) (string, error) {
	id := identity.Get(c.Request().Context())
	switch id.Identity.Type {
	case "User":
		if id.Identity.User != nil && id.Identity.User.UserID != "" {
			return id.Identity.User.UserID, nil
		}
	case "ServiceAccount":
		if id.Identity.ServiceAccount != nil && id.Identity.ServiceAccount.UserId != "" {
			return id.Identity.ServiceAccount.UserId, nil
		}
	}

	// Associate and other identity types commonly used in tests/dev may only have a username
	if id.Identity.User != nil {
		if id.Identity.User.UserID != "" {
			return id.Identity.User.UserID, nil
		}
		if id.Identity.User.Username != "" {
			return id.Identity.User.Username, nil
		}
	}

	return "", ce.NewErrorResponse(http.StatusBadRequest, "Missing user identity", "user id is required to manage preferences")
}

// readJSONStringBody decodes a JSON string body (e.g. `"true"`).
// Object bodies like {"value":"true"} are intentionally rejected.
func readJSONStringBody(c echo.Context) (string, error) {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(body, &value); err != nil {
		return "", err
	}
	return value, nil
}
