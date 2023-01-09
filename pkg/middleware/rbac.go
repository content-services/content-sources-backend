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
	client         client.Rbac
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

func NewRbac(config Rbac, proxy client.Rbac) echo.MiddlewareFunc {
	config.client = proxy
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := MatchedRoute(c)
			if config.Skipper != nil && config.Skipper(c) {
				log.Info().Msgf("path=%s skipped for rbac middleware", path)
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

			// FIXME Remove this trace
			log.Info().Msgf("RBAC:method=%s path=%s", method, path)

			// method := c.Request().Method
			// resource := fromPathToResource(path)
			// if resource == "" {
			// 	log.Error().Msgf("path=%s could not be mapped to any resource", path)
			// 	return echo.ErrUnauthorized
			// }

			// verb := fromHttpVerbToRbacVerb(method)
			// if verb == client.RbacVerbUndefined {
			// 	log.Error().Msgf("method=%s could not be mapped to any verb", method)
			// 	return echo.ErrUnauthorized
			// }
			resource, verb, err := config.PermissionsMap.Permission(method, path)
			if err != nil {
				log.Error().Msgf("Mapping not found for method=%s path=%s:%s", method, path, err.Error())
				return echo.ErrUnauthorized
			}

			// FIXME Remove this trace
			log.Info().Msgf("RBAC:Checking X-Rh-Identity")
			xrhid := c.Request().Header.Get(xrhidHeader)
			if xrhid == "" {
				log.Error().Msg("x-rh-identity header cannot be empty")
				return echo.ErrUnauthorized
			}

			// FIXME Remove this trace
			log.Info().Msgf("RBAC:x-rh-identity='%s'", xrhid)

			// FIXME Remove this trace
			log.Info().Msgf("RBAC:Checking resource=%s verb=%s", resource, verb)

			// TODO It is failing here
			allowed, err := config.client.Allowed(xrhid, resource, verb)

			if err != nil {
				// FIXME Remove this trace
				log.Info().Msgf("RBAC:Error checking permissions: %s", err.Error())
				log.Error().Msgf("error checking permissions: %s", err.Error())
				return echo.ErrUnauthorized
			}
			if !allowed {
				log.Info().Msgf("RBAC:request not allowed")
				log.Error().Msgf("request not allowed")
				return echo.ErrUnauthorized
			}

			// TODO Remove this trace
			log.Info().Str("path", path).Msg("request authorized for rbac")
			return next(c)
		}
	}
}

//
// Private functions
//

func fromHttpVerbToRbacVerb(httpMethod string) client.RbacVerb {
	switch httpMethod {
	case echo.GET:
		return client.RbacVerbRead
	case echo.POST:
		return client.RbacVerbWrite
	case echo.PATCH:
		return client.RbacVerbWrite
	case echo.PUT:
		return client.RbacVerbWrite
	case echo.DELETE:
		return client.RbacVerbWrite

	default:
		return client.RbacVerbUndefined
	}
}

func fromPathToResource(path string) string {
	// [/beta]/api/content-sources/v1/<resource>
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "-", "_")
	items := strings.Split(path, "/")
	if len(items) < 5 {
		return ""
	}
	items = items[1:]
	switch items[0] {
	case "beta":
		items = items[1:]
	}
	if items[0] != "api" {
		return ""
	}
	return items[3]
}
