package middleware

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

// WrapMiddleware wraps `func(http.Handler) http.Handler` into `echo.MiddlewareFunc`
func WrapMiddlewareWithSkipper(m func(http.Handler) http.Handler, skip echo_middleware.Skipper) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			if skip != nil && skip(c) {
				return next(c)
			}
			m(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.SetRequest(r)
				c.SetResponse(echo.NewResponse(w, c.Echo()))
				identityHeader := c.Request().Header.Get("X-Rh-Identity")
				if identityHeader != "" {
					c.Response().Header().Set("X-Rh-Identity", identityHeader)
				}
				err = next(c)
			})).ServeHTTP(c.Response(), c.Request())
			return
		}
	}
}

func SkipAuth(p string) bool {
	skipped := []string{
		"/ping",
		"/openapi.json",
		"/repository_gpg_key/:uuid",
	}
	if utils.Contains(skipped, p) || utils.Contains(skipped, strings.TrimSuffix(p, "/")) {
		return true
	}
	return false
}

func SkipRbac(c echo.Context, p string) bool {
	xrhid := identity.GetIdentity(c.Request().Context())
	skipped := []string{
		"/templates/:template_uuid/config.repo",
	}
	if utils.Contains(skipped, p) && xrhid.Identity.Type == "System" {
		return true
	}
	return false
}

func SkipMiddleware(c echo.Context) bool {
	p := MatchedRoute(c)
	// skip middleware for unregistered routes
	if p == "" {
		return true
	}

	lengthOfPrefix := len(strings.Split(api.FullRootPath(), "/"))
	splitPath := strings.Split(p, "/")

	// strip only the endpoint after the prefix from the matched route (i.e. /templates/:template_uuid/config.repo)
	if len(splitPath) > lengthOfPrefix {
		p = "/" + strings.Join(splitPath[lengthOfPrefix:], "/")
	}

	if SkipRbac(c, p) || SkipAuth(p) {
		return true
	}
	return false
}

func EnforceOrgId(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		xRHID := identity.GetIdentity(c.Request().Context())

		if xRHID.Identity.Internal.OrgID == config.RedHatOrg || xRHID.Identity.OrgID == config.RedHatOrg {
			err := ce.NewErrorResponse(http.StatusForbidden, "Invalid org ID", "Org ID cannot be -1")
			c.Error(err)
			return nil
		} else if xRHID.Identity.Internal.OrgID == config.CommunityOrg || xRHID.Identity.OrgID == config.CommunityOrg {
			err := ce.NewErrorResponse(http.StatusForbidden, "Invalid org ID", "Org ID cannot be -2")
			c.Error(err)
			return nil
		}

		return next(c)
	}
}
