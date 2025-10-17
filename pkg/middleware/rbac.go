package middleware

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	path_util "github.com/content-services/content-sources-backend/pkg/handler/utils"
	"github.com/content-services/content-sources-backend/pkg/rbac"
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

type Rbac struct {
	BaseUrl        string
	Skipper        echo_middleware.Skipper
	RbacClient     rbac.ClientWrapper
	KesselClient   rbac.ClientWrapper
	PermissionsMap *rbac.PermissionsMap
}

func NewRbac(rbacConfig Rbac) echo.MiddlewareFunc {
	if rbacConfig.PermissionsMap == nil {
		panic("PermissionsMap cannot be nil")
	}
	if rbacConfig.RbacClient == nil {
		panic("client cannot be nil")
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var err error
			logger := zerolog.Ctx(c.Request().Context())
			path := MatchedRoute(c)
			if rbacConfig.Skipper != nil && rbacConfig.Skipper(c) {
				return next(c)
			}

			if path = strings.Join(path_util.NewPathWithString(path).RemovePrefixes(), "/"); path == "" {
				return echo.ErrBadRequest
			}
			method := c.Request().Method

			resource, verb, err := rbacConfig.PermissionsMap.Permission(method, path)
			if err != nil {
				logger.Debug().Err(err).Msgf("No mapping found for method=%s path=%s", method, path)
				return echo.ErrNotFound
			}

			xrhid := c.Request().Header.Get(xrhidHeader)
			if xrhid == "" {
				logger.Debug().Msg("x-rh-identity is required")
				return echo.ErrUnauthorized
			}

			var allowed bool
			if config.FeatureAccessible(c.Request().Context(), config.Get().Features.Kessel) {
				logger.Debug().Msg("using kessel")
				allowed, err = rbacConfig.KesselClient.Allowed(c.Request().Context(), resource, verb)
			} else {
				logger.Debug().Msg("using rbac")
				allowed, err = rbacConfig.RbacClient.Allowed(c.Request().Context(), resource, verb)
			}

			if err != nil {
				logger.Error().Err(err).Msg("error checking permissions")
				return echo.ErrUnauthorized
			}
			if !allowed {
				logger.Debug().Msg("request not allowed")
				return echo.ErrUnauthorized
			}

			return next(c)
		}
	}
}
