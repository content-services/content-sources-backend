package middleware

import (
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
)

// This middleware will add rbac feature to the service

// https://echo.labstack.com/cookbook/middleware/
// https://github.com/labstack/echo/tree/master/middleware

const clientTimeout = 10 * time.Second
const xrhidHeader = "X-Rh-Identity"

type Rbac struct {
	BaseUrl string
	Skipper echo_middleware.Skipper

	client client.Rbac
}

func NewRbac(config Rbac, proxy client.Rbac) echo.MiddlewareFunc {
	config.client = proxy
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper != nil && config.Skipper(c) {
				return next(c)
			}

			resource := fromPathToResource(c.Request().URL.Path)
			if resource == "" {
				return echo.ErrUnauthorized
			}

			verb := fromHttpVerbToRbacVerb(c.Request().Method)
			if verb == client.RbacVerbUndefined {
				return echo.ErrUnauthorized
			}

			xrhid := c.Request().Header.Get(xrhidHeader)
			if xrhid == "" {
				return echo.ErrUnauthorized
			}

			allowed, err := config.client.Allowed(xrhid, resource, verb)
			if err != nil {
				return echo.ErrInternalServerError
			}
			if !allowed {
				return echo.ErrUnauthorized
			}

			return echo.ErrUnauthorized
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
