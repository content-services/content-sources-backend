package sonatype_bridge

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type healthResponse struct {
	Healthy  bool          `json:"healthy"`
	S3       serviceHealth `json:"s3"`
	Registry serviceHealth `json:"registry"`
}

type serviceHealth struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

// health is a readiness check: it probes external dependencies (the partner
// S3 bucket and the Lightwell registry) and returns 503 when either is
// unreachable. A failure means the bridge cannot do useful work, but
// restarting the process would not help — do not use this as a liveness probe.
func (h *adminHandler) health(c echo.Context) error {
	ctx := c.Request().Context()

	s3Err := h.s3.Ping(ctx)
	registryErr := h.registry.Ping(ctx)

	healthy := s3Err == nil && registryErr == nil

	result := healthResponse{
		Healthy:  healthy,
		S3:       serviceHealth{Reachable: s3Err == nil},
		Registry: serviceHealth{Reachable: registryErr == nil},
	}

	if s3Err != nil {
		result.S3.Error = s3Err.Error()
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
