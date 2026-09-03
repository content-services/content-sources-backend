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

// health is a readiness check: it probes external dependencies (JFrog
// Artifactory and the Lightwell registry) and returns 503 when either is
// unreachable. A failure means the bridge cannot do useful work, but
// restarting the process would not help — do not use this as a liveness probe.
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
