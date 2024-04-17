package middleware

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
	uuid2 "github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Adds the request Id to the general context for use by pulp_client, candlepin_client, and general passing to Doa Layer
// Note that Lecho already adds it to the logger via the lecho middleware
func AddRequestId(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		reqId := c.Request().Header.Get(config.HeaderRequestId)
		if reqId == "" {
			reqId = uuid2.NewString()
		}
		c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), config.ContextRequestIDKey{}, reqId)))
		return next(c)
	}
}
