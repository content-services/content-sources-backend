package middleware

import (
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"
)

// This middleware will add rbac feature to the service

// https://echo.labstack.com/cookbook/middleware/
// https://github.com/labstack/echo/tree/master/middleware

const clientTimeout = 10 * time.Second
const xrhidHeader = "X-Rh-Identity"
const application = "content-sources"

type rbacEntry struct {
	resource string
	verb     string
}

type rbacMappingMethods map[string]rbacEntry

type rbacMapping struct {
	paths map[string]rbacMappingMethods
}

func NewRbacMapping() *rbacMapping {
	return &rbacMapping{
		paths: map[string]rbacMappingMethods{},
	}
}

func (r *rbacMapping) Add(path string, method string, resource string, verb string) *rbacMapping {
	if mappedPath, ok := r.paths[path]; ok {
		if mappedMethod, ok := mappedPath[method]; ok {
			mappedMethod.resource = resource
			mappedMethod.verb = verb
		} else {
			mappedPath[method] = rbacEntry{
				resource: resource,
				verb:     verb,
			}
		}
	} else {
		r.paths[path] = rbacMappingMethods{}
	}
	return r
}

type Rbac struct {
	BaseUrl string
	Skipper echo_middleware.Skipper

	client client.Rbac
}

func NewRbac(config Rbac, proxy client.Rbac) echo.MiddlewareFunc {
	config.client = proxy
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if config.Skipper != nil && config.Skipper(c) {
				log.Info().Msgf("path=%s skipped for rbac middleware", path)
				return next(c)
			}

			resource := fromPathToResource(path)
			if resource == "" {
				log.Error().Msgf("path=%s could not be mapped to any resource", path)
				return echo.ErrUnauthorized
			}

			method := c.Request().Method
			verb := fromHttpVerbToRbacVerb(method)
			if verb == client.RbacVerbUndefined {
				log.Error().Msgf("method=%s could not be mapped to any verb", method)
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
