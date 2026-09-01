package jfrog_bridge

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type healthResponse struct {
	Healthy  bool          `json:"healthy"`
	JFrog    serviceHealth `json:"jfrog"`
	Registry serviceHealth `json:"registry"`
}

type serviceHealth struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

func (h *adminHandler) health(c echo.Context) error {
	ctx := c.Request().Context()

	jfrogErr := h.jfrog.Ping(ctx)
	registryErr := h.registry.Ping(ctx)

	healthy := jfrogErr == nil && registryErr == nil

	result := healthResponse{
		Healthy:  healthy,
		JFrog:    serviceHealth{Reachable: jfrogErr == nil},
		Registry: serviceHealth{Reachable: registryErr == nil},
	}

	if jfrogErr != nil {
		result.JFrog.Error = jfrogErr.Error()
	}
	if registryErr != nil {
		result.Registry.Error = registryErr.Error()
	}

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	return c.JSON(status, result)
}
