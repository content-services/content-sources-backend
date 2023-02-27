package middleware

import "github.com/labstack/echo/v4"

// See: https://github.com/labstack/echo/pull/1502/files
// This method exist for v5 echo framework
func MatchedRoute(ctx echo.Context) string {
	pathx := ctx.Path()
	for _, r := range ctx.Echo().Routes() {
		if pathx == r.Path {
			return r.Path
		}
	}
	return ""
}
