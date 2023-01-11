package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureEcho(t *testing.T) {

	type MethodPath struct {
		Method string
		Path   string
	}
	type TestCaseExpected map[string]map[string]string

	testCases := TestCaseExpected{
		"/ping": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.ping",
		},
		"/ping/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.ping",
		},
		"/api/content-sources/v1/ping/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.ping",
		},
		"/api/content-sources/v1.0/ping/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.ping",
		},
		"/api/content-sources/v1/openapi.json": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.openapi",
		},
		"/api/content-sources/v1.0/openapi.json": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.openapi",
		},
		"/api/content-sources/v1/repositories/": {
			"GET":  "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).listRepositories-fm",
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).createRepository-fm",
		},
		"/api/content-sources/v1.0/repositories/": {
			"GET":  "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).listRepositories-fm",
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).createRepository-fm",
		},
		"/api/content-sources/v1/popular_repositories/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.(*PopularRepositoriesHandler).listPopularRepositories-fm",
		},
		"/api/content-sources/v1.0/popular_repositories/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.(*PopularRepositoriesHandler).listPopularRepositories-fm",
		},
		"/api/content-sources/v1/repository_parameters/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryParameterHandler).listParameters-fm",
		},
		"/api/content-sources/v1.0/repository_parameters/": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryParameterHandler).listParameters-fm",
		},
		"/api/content-sources/v1/repositories/:uuid/rpms": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryRpmHandler).listRepositoriesRpm-fm",
		},
		"/api/content-sources/v1.0/repositories/:uuid/rpms": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryRpmHandler).listRepositoriesRpm-fm",
		},
		"/api/content-sources/v1/rpms/names": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryRpmHandler).searchRpmByName-fm",
		},
		"/api/content-sources/v1.0/rpms/names": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryRpmHandler).searchRpmByName-fm",
		},
		"/api/content-sources/v1/repositories/:uuid": {
			"PATCH":  "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).partialUpdate-fm",
			"PUT":    "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).fullUpdate-fm",
			"GET":    "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).fetch-fm",
			"DELETE": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).deleteRepository-fm",
		},
		"/api/content-sources/v1.0/repositories/:uuid": {
			"PATCH":  "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).partialUpdate-fm",
			"PUT":    "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).fullUpdate-fm",
			"GET":    "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).fetch-fm",
			"DELETE": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).deleteRepository-fm",
		},
		"/api/content-sources/v1/repository_parameters/external_gpg_key/": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryParameterHandler).fetchGpgKey-fm",
		},
		"/api/content-sources/v1.0/repository_parameters/external_gpg_key/": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryParameterHandler).fetchGpgKey-fm",
		},
		"/api/content-sources/v1/repositories/bulk_create/": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).bulkCreateRepositories-fm",
		},
		"/api/content-sources/v1.0/repositories/bulk_create/": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).bulkCreateRepositories-fm",
		},
		"/api/content-sources/v1/repository_parameters/validate/": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryParameterHandler).validate-fm",
		},
		"/api/content-sources/v1.0/repository_parameters/validate/": {
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryParameterHandler).validate-fm",
		},
	}

	e := ConfigureEcho(true)
	require.NotNil(t, e)

	// Match Routes in expected
	for _, route := range e.Routes() {
		t.Logf("Method=%s Path=%s Name=%s", route.Method, route.Path, route.Name)
		methods, okPath := testCases[route.Path]
		require.True(t, okPath)
		name, okMethod := methods[route.Method]
		require.True(t, okMethod)
		assert.Equal(t, name, route.Name)
	}
}
