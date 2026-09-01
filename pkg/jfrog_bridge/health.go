package jfrog_bridge

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *adminHandler) health(c echo.Context) error {
	ctx := c.Request().Context()

	jfrogErr := h.jfrog.Ping(ctx)
	registryErr := h.registry.Ping(ctx)

	healthy := jfrogErr == nil && registryErr == nil

	result := map[string]interface{}{
		"healthy": healthy,
		"jfrog": map[string]interface{}{
			"reachable": jfrogErr == nil,
		},
		"registry": map[string]interface{}{
			"reachable": registryErr == nil,
		},
	}

	if jfrogErr != nil {
		result["jfrog"].(map[string]interface{})["error"] = jfrogErr.Error()
	}
	if registryErr != nil {
		result["registry"].(map[string]interface{})["error"] = registryErr.Error()
	}

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	return c.JSON(status, result)
}
