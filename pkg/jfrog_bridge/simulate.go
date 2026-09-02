package jfrog_bridge

import (
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type simulateHandler struct {
	bridgeHandler *BridgeHandler
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

// RegisterJFrogBridgeRoutes adds the simulate and status routes to the
// admin API group, following the RegisterAdminNotificationsRoutes pattern.
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
		log.Error().Err(err).Msg("simulate: failed to create evidence creator")
		return
	}

	bh := NewBridgeHandler(registry, jfrog, evidence, metrics)
	bh.registryURL = cfg.RegistryURL
	h := &simulateHandler{bridgeHandler: bh}

	engine.Add(http.MethodPost, "/admin/jfrog_bridge/simulate", h.simulate, checkBridgeEnabled, checkAdminJfrogUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodPost, "/admin/jfrog_bridge/simulate",
		rbac.ResourceRepositories, rbac.RbacVerbWrite)

	engine.Add(http.MethodGet, "/admin/jfrog_bridge/status", h.status, checkBridgeEnabled, checkAdminJfrogUploadAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/jfrog_bridge/status",
		rbac.ResourceRepositories, rbac.RbacVerbRead)
}

func (h *simulateHandler) simulate(c echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error reading body", err.Error())
	}
	defer c.Request().Body.Close()

	remediations, err := ParseRemediations(body)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error parsing remediations", err.Error())
	}

	var results []map[string]string
	for _, rem := range remediations {
		gav := gavKey(rem)
		if err := h.bridgeHandler.processRemediation(c.Request().Context(), rem); err != nil {
			log.Error().Err(err).Str("gav", gav).Msg("simulate pipeline failed")
			results = append(results, map[string]string{
				"gav":    gav,
				"status": "failed",
				"error":  err.Error(),
			})
			continue
		}
		results = append(results, map[string]string{
			"gav":    gav,
			"status": "success",
		})
	}

	return c.JSON(http.StatusOK, results)
}

func (h *simulateHandler) status(c echo.Context) error {
	cfg := config.Get().JFrogBridge
	status := map[string]interface{}{
		"enabled":          cfg.Enabled,
		"catalog_url":      cfg.CatalogURL,
		"catalog_repo":     cfg.CatalogRepo,
		"registry_url":     cfg.RegistryURL,
		"registry_osv_url": cfg.RegistryOSVURL,
	}
	return c.JSON(http.StatusOK, status)
}
