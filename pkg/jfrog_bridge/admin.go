package jfrog_bridge

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
	jfrog         JFrogClient
}

func checkBridgeEnabled(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.Get().JFrogBridge.Enabled {
			return ce.NewErrorResponse(http.StatusBadRequest,
				"JFrog bridge is disabled", "Set jfrog_bridge.enabled to true")
		}
		return next(c)
	}
}

func checkAdminJfrogUploadAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		feature := config.Get().Features.AdminJfrogUpload
		if !feature.Enabled {
			return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access JFrog upload",
				"Admin JFrog upload feature is disabled.")
		}
		if !config.FeatureAccessible(c.Request().Context(), feature) {
			return ce.NewErrorResponse(http.StatusBadRequest, "Cannot access JFrog upload",
				"Neither the user nor account is allowed.")
		}
		return next(c)
	}
}

// RegisterJFrogBridgeRoutes adds the admin routes for the JFrog bridge:
// simulate, status, and health.
func RegisterJFrogBridgeRoutes(engine *echo.Group) {
	if engine == nil {
		panic("engine is nil")
	}

	cfg := loadConfig()
	if !cfg.Enabled {
		return
	}

	metrics := getSharedMetrics()

	registry := newRegistryClient(cfg)
	jfrog := newJFrogClient(cfg)
	evidence, err := newEvidenceCreator(cfg)
	if err != nil {
		log.Error().Err(err).Msg("admin: failed to create evidence creator")
		return
	}

	bh := NewBridgeHandler(registry, jfrog, evidence, metrics)
	bh.registryURL = cfg.RegistryURL
	h := &adminHandler{bridgeHandler: bh, registry: registry, jfrog: jfrog}

	engine.Add(http.MethodPost, "/admin/jfrog_bridge/simulate", h.simulate, checkBridgeEnabled, checkAdminJfrogUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodPost, "/admin/jfrog_bridge/simulate",
		rbac.ResourceRepositories, rbac.RbacVerbWrite)

	engine.Add(http.MethodGet, "/admin/jfrog_bridge/status", h.status, checkBridgeEnabled, checkAdminJfrogUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/jfrog_bridge/status",
		rbac.ResourceRepositories, rbac.RbacVerbRead)

	engine.Add(http.MethodGet, "/admin/jfrog_bridge/health", h.health, checkBridgeEnabled, checkAdminJfrogUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/jfrog_bridge/health",
		rbac.ResourceRepositories, rbac.RbacVerbRead)
}
