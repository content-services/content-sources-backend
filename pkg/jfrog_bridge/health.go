package jfrog_bridge

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type serviceHealth struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

func newServiceHealth(err error) serviceHealth {
	if err != nil {
		return serviceHealth{Reachable: false, Error: err.Error()}
	}
	return serviceHealth{Reachable: true}
}

// health is a readiness check: it probes the partner's external dependencies
// and returns 503 when any is unreachable. A failure means the bridge cannot
// do useful work, but restarting the process would not help — do not use
// this as a liveness probe.
func (h *adminHandler) health(c echo.Context) error {
	healthy, body := bridgeFromContext(c).partner.Health(c.Request().Context())

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}
	return c.JSON(status, body)
}
