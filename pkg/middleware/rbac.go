package middleware

import (
	"strings"

	path_util "github.com/content-services/content-sources-backend/pkg/handler/utils"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
)

// This middleware will add rbac feature to the service

// https://echo.labstack.com/cookbook/middleware/
// https://github.com/labstack/echo/tree/master/middleware

const (
	xrhidHeader = "X-Rh-Identity"
)

var skipRbacRoutes = []string{
	"/api/content-sources/v1.0/templates/:template_uuid/config.repo",
}

type Rbac struct {
	BaseUrl        string
	Skipper        echo_middleware.Skipper
	Client         rbac.ClientWrapper
	PermissionsMap *rbac.PermissionsMap
}

func NewRbac(config Rbac) echo.MiddlewareFunc {
	if config.PermissionsMap == nil {
		panic("PermissionsMap cannot be nil")
	}
	if config.Client == nil {
		panic("client cannot be nil")
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			logger := zerolog.Ctx(c.Request().Context())
			path := MatchedRoute(c)
			if config.Skipper != nil && config.Skipper(c) || utils.Contains(skipRbacRoutes, path) {
				return next(c)
			}

			if path = strings.Join(path_util.NewPathWithString(path).RemovePrefixes(), "/"); path == "" {
				return echo.ErrBadRequest
			}
			method := c.Request().Method

			resource, verb, err := config.PermissionsMap.Permission(method, path)
			if err != nil {
				logger.Error().Msgf("No mapping found for method=%s path=%s:%s", method, path, err.Error())
				return echo.ErrUnauthorized
			}

			xrhid := c.Request().Header.Get(xrhidHeader)
			if xrhid == "" {
				logger.Error().Msg("x-rh-identity is required")
				return echo.ErrBadRequest
			}

			allowed, err := config.Client.Allowed(c.Request().Context(), resource, verb)

			if err != nil {
				logger.Error().Msgf("error checking permissions: %s", err.Error())
				return echo.ErrUnauthorized
			}
			if !allowed {
				logger.Error().Msgf("request not allowed")
				return echo.ErrUnauthorized
			}

			return next(c)
		}
	}
}
