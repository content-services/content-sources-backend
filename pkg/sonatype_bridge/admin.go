package sonatype_bridge

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type adminHandler struct {
	bridgeHandler *BridgeHandler
	registry      RegistryClient
	s3            S3Uploader
}

func checkBridgeEnabled(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.Get().SonatypeBridge.Enabled {
			return ce.NewErrorResponse(http.StatusBadRequest,
				"Sonatype bridge is disabled", "Set sonatype_bridge.enabled to true")
		}
		return next(c)
	}
}

func checkAdminSonatypeUploadAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		feature := config.Get().Features.AdminSonatypeUpload
		if !feature.Enabled {
			return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access Sonatype upload",
				"Admin Sonatype upload feature is disabled.")
		}
		if !config.FeatureAccessible(c.Request().Context(), feature) {
			return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access Sonatype upload",
				"Neither the user nor account is allowed.")
		}
		return next(c)
	}
}

// RegisterSonatypeBridgeRoutes adds the admin routes for the Sonatype bridge:
// simulate, status, and health.
func RegisterSonatypeBridgeRoutes(engine *echo.Group) {
	if engine == nil {
		panic("engine is nil")
	}

	cfg := loadConfig()
	if !cfg.Enabled {
		return
	}

	uploader, err := newS3Uploader(cfg)
	if err != nil {
		log.Error().Err(err).Msg("sonatype-bridge: admin routes not registered, partner S3 is not configured")
		return
	}

	metrics := getSharedMetrics()

	registry := newRegistryClient(cfg)

	bh := NewBridgeHandler(registry, uploader, metrics)
	h := &adminHandler{bridgeHandler: bh, registry: registry, s3: uploader}

	engine.Add(http.MethodPost, "/admin/sonatype_bridge/simulate", h.simulate, checkBridgeEnabled, checkAdminSonatypeUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodPost, "/admin/sonatype_bridge/simulate",
		rbac.ResourceRepositories, rbac.RbacVerbWrite)

	engine.Add(http.MethodGet, "/admin/sonatype_bridge/status", h.status, checkBridgeEnabled, checkAdminSonatypeUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/sonatype_bridge/status",
		rbac.ResourceRepositories, rbac.RbacVerbRead)

	engine.Add(http.MethodGet, "/admin/sonatype_bridge/health", h.health, checkBridgeEnabled, checkAdminSonatypeUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/sonatype_bridge/health",
		rbac.ResourceRepositories, rbac.RbacVerbRead)
}
