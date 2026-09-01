package jfrog_bridge

import (
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

func (h *adminHandler) simulate(c echo.Context) error {
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

func (h *adminHandler) status(c echo.Context) error {
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
