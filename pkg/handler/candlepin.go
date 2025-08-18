package handler

import (
	"errors"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

var RHProductIDs = []string{"479"}

type CandlepinHandler struct {
	cpClient candlepin_client.CandlepinClient
	cache    cache.Cache
}

func RegisterCandlepinRoutes(engine *echo.Group, cpClient *candlepin_client.CandlepinClient, cache *cache.Cache) {
	if engine == nil {
		panic("engine is nil")
	}
	if cpClient == nil {
		panic("cpClient is nil")
	}
	if cache == nil {
		panic("cache is nil")
	}

	candlepinHandler := CandlepinHandler{
		cpClient: *cpClient,
		cache:    *cache,
	}
	addRepoRoute(engine, http.MethodGet, "/subscription_check/", candlepinHandler.subscriptionCheck, rbac.RbacVerbRead)
}

func (h *CandlepinHandler) subscriptionCheck(c echo.Context) error {
	var resp api.SubscriptionCheckResponse
	_, orgID := getAccountIdOrgId(c)

	check, err := h.cache.GetSubscriptionCheck(c.Request().Context())
	if err != nil && !errors.Is(err, cache.ErrNotFound) {
		log.Logger.Error().Err(err).Msg("subscriptionCheck: error reading from cache")
	}
	if check != nil {
		return c.JSON(http.StatusOK, check)
	}

	product, err := h.cpClient.ListProducts(c.Request().Context(), orgID, RHProductIDs)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "subscription check error", err.Error())
	}
	if len(product) == 0 {
		resp.RedHatEnterpriseLinux = false
	} else {
		resp.RedHatEnterpriseLinux = true
		err = h.cache.SetSubscriptionCheck(c.Request().Context(), resp)
		if err != nil {
			log.Logger.Error().Err(err).Msg("subscriptionCheck: error writing to cache")
		}
	}

	return c.JSON(http.StatusOK, resp)
}
