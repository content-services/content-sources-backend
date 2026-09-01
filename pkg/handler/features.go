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
	"github.com/rs/zerolog/log"
)

type FeaturesHandler struct {
}

func RegisterFeaturesRoutes(engine *echo.Group) {
	fh := FeaturesHandler{}
	addRepoRoute(engine, http.MethodGet, "/features/", fh.listFeatures, rbac.RbacVerbRead)
}

// ListFeatures godoc
// @Summary      List Features within the application, whether they are enabled, and whether the requesting user can use them
// @ID           listFeatures
// @Description  Get features enables retrieving information about the features within an application, regardless of their current status (enabled or disabled) and the user's access to them.
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
			Accessible: config.FeatureAccessible(c.Request().Context(), feature),
		}
	}
	return c.JSON(http.StatusOK, set)
}

func CheckSnapshotAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.Snapshots.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Snapshotting Feature is disabled.", "")
	} else if config.FeatureAccessible(ctx, config.Get().Features.Snapshots) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage repository snapshots",
			"Neither the user nor the account is allowed.")
	}
}

func CheckAdminTaskAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.AdminTasks.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage admin tasks",
			"Admin tasks feature is disabled.")
	} else if config.FeatureAccessible(ctx, config.Get().Features.AdminTasks) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage admin tasks",
			"Neither the user nor account is allowed.")
	}
}

func CheckAdminPartnerRepositoriesAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.AdminPartnerRepositories.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot administer partner repositories",
			"Administration of Partner repositories feature is disabled.")
	} else if config.FeatureAccessible(ctx, config.Get().Features.AdminPartnerRepositories) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot administer partner repositories",
			"Neither the user nor account is allowed.")
	}
}

func CheckLightwellNotificationsAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.LightwellNotifications.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage lightwell notification preferences",
			"Lightwell notifications feature is disabled.")
	} else if config.FeatureAccessible(ctx, config.Get().Features.LightwellNotifications) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot manage lightwell notification preferences",
			"Neither the user nor account is allowed.")
	}
}

func CheckAdminNotificationsAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.AdminNotifications.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot send test notifications",
			"Admin notifications feature is disabled.")
	} else if config.FeatureAccessible(ctx, config.Get().Features.AdminNotifications) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot send test notifications",
			"Neither the user nor account is allowed.")
	}
}

func CheckLightwellBeaconAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.LightwellBeacon.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access Lightwell Beacon",
			"Lightwell Beacon is disabled.")
	} else if config.FeatureAccessible(ctx, config.Get().Features.LightwellBeacon) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access Lightwell Beacon",
			"Neither the user nor account is allowed.")
	}
}

func CheckLightwellLensAccessible(ctx context.Context) (err error) {
	if !config.Get().Features.LightwellLens.Enabled {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access Lightwell Lens",
			"Lightwell Lens is disabled.")
	} else if config.FeatureAccessible(ctx, config.Get().Features.LightwellLens) {
		return nil
	} else {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access Lightwell Lens",
			"Neither the user nor account is allowed.")
	}
}
