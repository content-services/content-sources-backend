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
}

func NewRbac(config Rbac) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			resource := fromPathToResource(c.Request().URL.Path)
			if resource == "" {
				return echo.ErrUnauthorized
			}

			verb := fromHttpVerbToRbacVerb(c.Request().Method)
			if verb == client.VerbUndefined {
				return echo.ErrUnauthorized
			}

			xrhid := c.Request().Header.Get(xrhidHeader)
			if xrhid == "" {
				return echo.ErrUnauthorized
			}

			rbac := client.NewRbac(config.BaseUrl, clientTimeout)
			allowed, err := rbac.Allowed(xrhid, resource, verb)
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
		return client.VerbRead

	case echo.POST:
		return client.VerbWrite
	case echo.PATCH:
		return client.VerbWrite
	case echo.PUT:
		return client.VerbWrite
	case echo.DELETE:
		return client.VerbWrite

	default:
		return client.VerbUndefined
	}
}

func fromPathToResource(path string) string {
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
	// [/beta]/api/content-sources/v1/resource
	return items[3]
}
