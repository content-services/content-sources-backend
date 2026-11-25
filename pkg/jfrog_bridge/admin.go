package jfrog_bridge

import (
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const bridgeContextKey = "bridgeHandler"

type adminHandler struct {
	bridges map[string]*BridgeHandler
}

// checkPartnerAccessible resolves the :partner path parameter. Unknown,
// disabled, or unconfigured partners return 404. The partner's own admin
// feature flag then gates access.
func (h *adminHandler) checkPartnerAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := c.Param("partner")
		p, known := lookupPartner(name)
		bh, configured := h.bridges[name]
		if !known || !configured || !p.enabled() {
			return ce.NewErrorResponse(http.StatusNotFound, "Bridge partner not found",
				fmt.Sprintf("No enabled bridge partner named %q", name))
		}

		feature := p.adminFeature()
		if !feature.Enabled {
			return ce.NewErrorResponse(http.StatusBadRequest, fmt.Sprintf("Cannot access %s upload", p.displayName),
				fmt.Sprintf("Admin %s upload feature is disabled.", p.displayName))
		}
		if !config.FeatureAccessible(c.Request().Context(), feature) {
			return ce.NewErrorResponse(http.StatusBadRequest, fmt.Sprintf("Cannot access %s upload", p.displayName),
				"Neither the user nor account is allowed.")
		}

		c.Set(bridgeContextKey, bh)
		return next(c)
	}
}

func bridgeFromContext(c echo.Context) *BridgeHandler {
	bh, _ := c.Get(bridgeContextKey).(*BridgeHandler)
	return bh
}

// RegisterBridgeRoutes adds the admin routes for every enabled bridge
// partner: /admin/bridge/:partner/{simulate,status,health}.
func RegisterBridgeRoutes(engine *echo.Group) {
	if engine == nil {
		panic("engine is nil")
	}

	metrics := getSharedMetrics()
	bridges := map[string]*BridgeHandler{}
	for _, p := range partners {
		if !p.enabled() {
			continue
		}
		handler, err := p.newHandler()
		if err != nil {
			log.Error().Err(err).Str("partner", p.name).Msg("content-bridge: admin routes not available for partner")
			continue
		}
		bridges[p.name] = NewBridgeHandler(handler, metrics)
	}
	if len(bridges) == 0 {
		return
	}

	h := &adminHandler{bridges: bridges}

	engine.Add(http.MethodPost, "/admin/bridge/:partner/simulate", h.simulate, h.checkPartnerAccessible)
	rbac.ServicePermissions.Add(http.MethodPost, "/admin/bridge/:partner/simulate",
		rbac.ResourceRepositories, rbac.RbacVerbWrite)

	engine.Add(http.MethodGet, "/admin/bridge/:partner/status", h.status, h.checkPartnerAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/bridge/:partner/status",
		rbac.ResourceRepositories, rbac.RbacVerbRead)

	engine.Add(http.MethodGet, "/admin/bridge/:partner/health", h.health, h.checkPartnerAccessible)
	rbac.ServicePermissions.Add(http.MethodGet, "/admin/bridge/:partner/health",
		rbac.ResourceRepositories, rbac.RbacVerbRead)
}
