package scathan_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestGetOSV(t *testing.T) {
	const body = `{"id":"CVE-2099-0001","schema_version":"1.6.0"}`
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/osv/CVE-2099-0001", r.URL.Path)
		// Identity must be forwarded so Scathan can enforce embargo/entitlement.
		assert.NotEmpty(t, r.Header.Get(api.IdentityHeader))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer httpServer.Close()

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.Scathan.Server = httpServer.URL

	s := scathanImpl{client: http.Client{}}

	record, statusCode, err := s.GetOSV(context.Background(), "CVE-2099-0001")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, body, string(record))
}

func TestGetOSVByCoordinates(t *testing.T) {
	const purl = "pkg:maven/org.example/widget@1.2.3"
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/coordinates", r.URL.Path)
		assert.Equal(t, purl, r.URL.Query().Get("purl"))
		assert.Equal(t, "osv", r.URL.Query().Get("format"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer httpServer.Close()

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.Scathan.Server = httpServer.URL

	s := scathanImpl{client: http.Client{}}

	_, statusCode, err := s.GetOSVByCoordinates(context.Background(), purl)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestHealthz(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/healthz", r.URL.Path)
		// Healthz is an unauthenticated probe: no identity should be forwarded.
		assert.Empty(t, r.Header.Get(api.IdentityHeader))
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.Scathan.Server = httpServer.URL

	s := scathanImpl{client: http.Client{}}

	statusCode, err := s.Healthz(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestGetOSVUnconfigured(t *testing.T) {
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.Scathan.Server = ""

	s := scathanImpl{client: http.Client{}}

	_, _, err := s.GetOSV(context.Background(), "CVE-2099-0001")
	assert.Error(t, err)
}
