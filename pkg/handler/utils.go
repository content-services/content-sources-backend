package handler

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
)

func GetHeader(c echo.Context, key string, defvalues []string) []string {
	val, ok := c.Request().Header[key]
	if !ok {
		return defvalues
	}
	return val
}

func removeEndSuffix(source string, suffix string) string {
	output := source
	j := len(source) - 1

	for j > 0 && strings.HasSuffix(output, suffix) {
		output = strings.TrimSuffix(output, suffix)
	}

	return output
}

func addRepoRoute(e *echo.Group, method string, path string, h echo.HandlerFunc, verb rbac.Verb, m ...echo.MiddlewareFunc) {
	e.Add(method, path, h, m...)
	rbac.ServicePermissions.Add(method, path, rbac.ResourceRepositories, verb)
}

func addTemplateRoute(e *echo.Group, method string, path string, h echo.HandlerFunc, verb rbac.Verb, m ...echo.MiddlewareFunc) {
	e.Add(method, path, h, m...)
	rbac.ServicePermissions.Add(method, path, rbac.ResourceTemplates, verb)
}

func preprocessInput(input *api.ContentUnitSearchRequest) {
	if input == nil {
		return
	}
	for i, url := range input.URLs {
		input.URLs[i] = removeEndSuffix(url, "/")
	}
	if input.Limit == nil {
		input.Limit = pointy.Int(api.ContentUnitSearchRequestLimitDefault)
	}
	if *input.Limit > api.ContentUnitSearchRequestLimitMaximum {
		*input.Limit = api.ContentUnitSearchRequestLimitMaximum
	}
}
