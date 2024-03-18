package middleware

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
)

// Adds the request Id to the general context
func AddRequestId(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Header.Get(config.HeaderRequestId) != "" {
			c.Set(config.HeaderRequestId, c.Request().Header.Get(config.HeaderRequestId))
		} else {
			c.Set(config.HeaderRequestId, c.Request().Header.Get(config.HeaderRequestId))
		}
		return next(c)
	}
}
