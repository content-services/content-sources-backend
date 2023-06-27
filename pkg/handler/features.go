package handler

import (
	"context"
	"net/http"
	"reflect"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
)

type FeaturesHandler struct {
}

func RegisterFeaturesRoutes(engine *echo.Group) {
	fh := FeaturesHandler{}
	addRoute(engine, http.MethodGet, "/features/", fh.listFeatures, rbac.RbacVerbRead)
}

// ListFeatures godoc
// @Summary      List Features within the application, whether they are enabled, and whether the requesting user can use them
// @ID           listFeatures
// @Description  Get features available for the user within their Organization
// @Tags         features
// @Accept       json
// @Produce      json
// @Success      200 {object} api.FeatureSet
// @Router       /features/ [get]
func (fh *FeaturesHandler) listFeatures(c echo.Context) error {
	set := make(api.FeatureSet)
	elem := reflect.ValueOf(config.Get().Features)

	for i := 0; i < elem.NumField(); i++ {
		name := strings.ToLower(elem.Type().Field(i).Name)
		value := elem.Field(i).Interface()
		feature, valid := value.(config.Feature)
		if !valid {
			log.Logger.Error().Msgf("Could not load feature %v", feature)
			continue
		}
		set[name] = api.Feature{
			Enabled:    feature.Enabled,
			Accessible: accessible(c.Request().Context(), feature),
		}
	}
	return c.JSON(http.StatusOK, set)
}

func accessible(ctx context.Context, feature config.Feature) bool {
	if feature.Accounts == nil && feature.Users == nil {
		return true
	}
	identity := identity.Get(ctx)
	if feature.Accounts != nil && slices.Contains(*feature.Accounts, identity.Identity.AccountNumber) {
		return true
	}
	if feature.Users != nil && slices.Contains(*feature.Users, identity.Identity.User.Username) {
		return true
	}
	return false
}

func CheckSnapshotAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.Snapshots.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Snapshotting Feature is disabled.", "")
	} else if accessible(ctx, config.Get().Features.Snapshots) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage repository snapshots",
			"Neither the user nor account is not allowed.")
	}
}

func CheckAdminTaskAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.AdminTasks.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage admin tasks",
			"Admin tasks feature is disabled.")
	} else if accessible(ctx, config.Get().Features.AdminTasks) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage admin tasks",
			"Neither the user nor account is allowed.")
	}
}
