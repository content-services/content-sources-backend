package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
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

// func CheckOrgID(c echo.Context, identityHeader string) error {
// 	decodedBytes, err := base64.StdEncoding.DecodeString(identityHeader)
// 	if err != nil {
// 		return ce.NewErrorResponse(http.StatusInternalServerError, "Error decoding base64", "Cannot decode identity")
// 	}
// 	var xRHID identity.XRHID
// 	err = json.Unmarshal(decodedBytes, &xRHID)
// 	if err != nil {
// 		return ce.NewErrorResponse(http.StatusInternalServerError, "Error unmarshaling json", "Cannot unmarshal identity")
// 	}

// 	if xRHID.Identity.Internal.OrgID == "-1" {
// 		return ce.NewErrorResponse(http.StatusForbidden, "Request not allowed", "Org ID cannot be -1.")
// 	}

// 	return nil
// }

func EnforceOrgId(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		identityHeader := c.Request().Header.Get("X-Rh-Identity")
		decodedBytes, err := base64.StdEncoding.DecodeString(identityHeader)
		if err != nil {
			c.Error(err)
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error decoding base64", "Cannot decode identity")
		}
		var xRHID identity.XRHID
		err = json.Unmarshal(decodedBytes, &xRHID)
		if err != nil {
			c.Error(err)
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error unmarshaling json", "Cannot unmarshal identity")
		}

		if xRHID.Identity.Internal.OrgID == "-1" {
			err = ce.NewErrorResponse(http.StatusForbidden, "Request not allowed", "Org ID cannot be -1.")
			c.Error(err)
			return err
		}

		return next(c)
	}
}
