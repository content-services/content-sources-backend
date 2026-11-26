package pulp_client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func usePulpTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	pulp := &config.Get().Clients.Pulp
	orig := *pulp
	t.Cleanup(func() { config.Get().Clients.Pulp = orig })
	pulp.Server = srv.URL
	pulp.Username = "admin"
	pulp.Password = "password"
	pulp.ClientCert = ""
	pulp.ClientKey = ""
	pulp.CACert = ""
	pulp.ClientCertPath = ""
	pulp.ClientKeyPath = ""
	pulp.CACertPath = ""
}

func TestListPythonPackageContentPagesAndFilters(t *testing.T) {
	base := "/api/pulp/lightwell/api/v3/repositories/python/python/0190/versions/3/"
	target := "/api/pulp/lightwell/api/v3/repositories/python/python/0190/versions/5/"
	var queries []url.Values
	usePulpTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query())
		assert.Equal(t, "/api/pulp/lightwell/api/v3/content/python/packages/", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("offset") {
		case "0":
			// The offset advances by the rows received, not by the requested limit.
			_, _ = w.Write([]byte(`{"count":2,"next":"/api/pulp/lightwell/api/v3/content/python/packages/?offset=1","previous":null,"results":[{"name":"Django","version":"5.0"}]}`))
		case "1":
			_, _ = w.Write([]byte(`{"count":2,"next":null,"previous":null,"results":[{"name":"requests","version":"2.0"}]}`))
		default:
			http.Error(w, "unexpected offset", http.StatusBadRequest)
		}
	})

	impl := getPulpImpl()
	impl.domainName = "lightwell"
	var got []string
	err := impl.ListPythonPackageContent(context.Background(), PythonPackageContentFilters{
		AddedBetween: []string{base, target},
		NameIn:       []string{"django", "requests"},
		Fields:       []string{"name", "version"},
	}, func(rows []zest.PythonPythonPackageContentResponse) error {
		for _, row := range rows {
			got = append(got, row.GetName()+"@"+row.GetVersion())
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Django@5.0", "requests@2.0"}, got)
	require.Len(t, queries, 2)
	q := queries[0]
	assert.Equal(t, base+","+target, q.Get("added_between"))
	assert.Empty(t, q.Get("removed_between"))
	assert.Empty(t, q.Get("repository_version"))
	assert.Equal(t, "django,requests", q.Get("name__in"))
	assert.Equal(t, []string{"name", "version"}, q["fields"])
	assert.Equal(t, "pk", q.Get("ordering"))
	assert.Equal(t, fmt.Sprint(pythonPackageContentPageSize), q.Get("limit"))
}

func TestListPythonPackageContentStopsOnCallbackError(t *testing.T) {
	var calls int
	usePulpTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":5000,"next":"/next/","previous":null,"results":[{"name":"a","version":"1"}]}`))
	})

	impl := getPulpImpl()
	impl.domainName = "lightwell"
	err := impl.ListPythonPackageContent(context.Background(), PythonPackageContentFilters{
		RepositoryVersion: "/api/pulp/lightwell/api/v3/repositories/python/python/0190/versions/5/",
	}, func([]zest.PythonPythonPackageContentResponse) error {
		return fmt.Errorf("stop")
	})
	assert.EqualError(t, err, "stop")
	assert.Equal(t, 1, calls)
}

func TestPythonPackageContentFiltersValidate(t *testing.T) {
	v := "/versions/1/"
	assert.NoError(t, PythonPackageContentFilters{RepositoryVersion: v}.validate())
	assert.NoError(t, PythonPackageContentFilters{AddedBetween: []string{v, v}}.validate())
	assert.NoError(t, PythonPackageContentFilters{RemovedBetween: []string{v, v}}.validate())
	assert.Error(t, PythonPackageContentFilters{}.validate())
	assert.Error(t, PythonPackageContentFilters{NameIn: []string{"a"}}.validate())
	assert.Error(t, PythonPackageContentFilters{RepositoryVersion: v, AddedBetween: []string{v, v}}.validate())
	assert.Error(t, PythonPackageContentFilters{AddedBetween: []string{v}}.validate())
	assert.Error(t, PythonPackageContentFilters{RemovedBetween: []string{v, v, v}}.validate())
}
