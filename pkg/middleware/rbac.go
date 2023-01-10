package middleware

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"
)

// This middleware will add rbac feature to the service

// https://echo.labstack.com/cookbook/middleware/
// https://github.com/labstack/echo/tree/master/middleware

const (
	xrhidHeader = "X-Rh-Identity"
	application = "content-sources"
)

type rbacEntry struct {
	resource string
	verb     client.RbacVerb
}

// PermissionsMap Map Method and Path to a RbacEntry
type PermissionsMap map[string]map[string]rbacEntry

func NewPermissionsMap() *PermissionsMap {
	return &PermissionsMap{}
}

func (pm *PermissionsMap) Add(method string, path string, res string, verb client.RbacVerb) *PermissionsMap {
	// Avoid using empty strings
	if method == "" || path == "" || res == "" || verb == "" {
		return nil
	}
	// Avoid using of wildcard during setting the permissions map
	if res == "*" || verb == "*" {
		return nil
	}
	if paths, ok := (*pm)[method]; ok {
		if permission, ok := paths[path]; ok {
			permission.resource = res
			permission.verb = verb
		} else {
			paths[path] = rbacEntry{
				resource: res,
				verb:     verb,
			}
		}
	} else {
		(*pm)[method] = map[string]rbacEntry{
			path: {
				resource: res,
				verb:     verb,
			},
		}
	}
	return pm
}

func (pm *PermissionsMap) Permission(method string, path string) (res string, verb client.RbacVerb, err error) {
	if paths, ok := (*pm)[method]; ok {
		if permission, ok := paths[path]; ok {
			return permission.resource, permission.verb, nil
		}
	}
	return "", "", fmt.Errorf("no permission found for method=%s and path=%s", method, path)
}

type Rbac struct {
	BaseUrl        string
	Skipper        echo_middleware.Skipper
	Client         client.Rbac
	PermissionsMap *PermissionsMap
}

// See: https://github.com/labstack/echo/pull/1502/files
func MatchedRoute(c echo.Context) string {
	pathx := c.Path()
	for _, r := range c.Echo().Routes() {
		if pathx == r.Path {
			return r.Path
		}
	}
	return ""
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
			path := MatchedRoute(c)
			if config.Skipper != nil && config.Skipper(c) {
				return next(c)
			}

			pathItems := strings.Split(path, "/")
			if pathItems[0] == "" {
				pathItems = pathItems[1:]
			}
			if pathItems[0] == "beta" {
				pathItems = pathItems[1:]
			}

			if pathItems[0] != "api" {
				return echo.ErrBadRequest
			}
			pathItems = pathItems[1:]

			if pathItems[0] != application {
				return echo.ErrBadRequest
			}
			pathItems = pathItems[1:]

			if pathItems[0][0] != 'v' {
				return echo.ErrBadRequest
			}
			pathItems = pathItems[1:]

			for idx, path := range pathItems {
				if path == "" {
					pathItems = pathItems[0:idx]
					break
				}
			}

			path = strings.Join(pathItems, "/")
			method := c.Request().Method

			resource, verb, err := config.PermissionsMap.Permission(method, path)
			if err != nil {
				log.Error().Msgf("No mapping found for method=%s path=%s:%s", method, path, err.Error())
				return echo.ErrUnauthorized
			}

			xrhid := c.Request().Header.Get(xrhidHeader)
			if xrhid == "" {
				log.Error().Msg("x-rh-identity header cannot be empty")
				return echo.ErrUnauthorized
			}

			allowed, err := config.Client.Allowed(xrhid, resource, verb)

			if err != nil {
				log.Error().Msgf("error checking permissions: %s", err.Error())
				return echo.ErrUnauthorized
			}
			if !allowed {
				log.Error().Msgf("request not allowed")
				return echo.ErrUnauthorized
			}

			return next(c)
		}
	}
}
