package middleware

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
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

func SkipAuth(c echo.Context) bool {
	p := c.Request().URL.Path
	lengthOfPrefix := len(strings.Split(api.FullRootPath(), "/"))
	splitPath := strings.Split(p, "/")

	skipped := []string{"ping", "openapi.json"}
	for i := 0; i < len(skipped); i++ {
		path := skipped[i]

		if p == "/"+path || p == "/"+path+"/" {
			return true
		}
		if strings.HasPrefix(p, "/api/"+config.DefaultAppName+"/") &&
			len(splitPath) == 5 &&
			splitPath[4] == path {
			return true
		}
	}

	// skip endpoint repository_gpg_key/:uuid
	lengthOfSkipPath := len(strings.Split("repository_gpg_key/*", "/"))
	lengthOfPath := len(strings.Split(p, "/"))
	if strings.HasPrefix(p, "/api/"+config.DefaultAppName+"/") {
		if lengthOfPrefix+lengthOfSkipPath == lengthOfPath && splitPath[4] == "repository_gpg_key" {
			return true
		}
	}

	return false
}

func EnforceOrgId(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		xRHID := identity.Get(c.Request().Context())

		if xRHID.Identity.Internal.OrgID == "-1" || xRHID.Identity.OrgID == "-1" {
			//err := ce.NewErrorResponse(http.StatusForbidden, "Invalid org ID", "Org ID cannot be -1") // this causes a 500 in the test only, not sure why
			err := echo.ErrForbidden
			c.Error(err)
			c.Request().Context().Done()
			return nil
		}

		return next(c)
	}
}
