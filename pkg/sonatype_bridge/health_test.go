package sonatype_bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHandler_AllHealthy(t *testing.T) {
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer registryServer.Close()

	uploader := newTestS3Uploader(s3Server, "")
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{s3: uploader, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/sonatype_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Healthy)
	assert.True(t, body.S3.Reachable)
	assert.True(t, body.Registry.Reachable)
}

func TestHealthHandler_S3Down(t *testing.T) {
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s3Server.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer registryServer.Close()

	uploader := newTestS3Uploader(s3Server, "")
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{s3: uploader, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/sonatype_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Healthy)
	assert.False(t, body.S3.Reachable)
	assert.NotEmpty(t, body.S3.Error)
	assert.True(t, body.Registry.Reachable)
}

func TestHealthHandler_RegistryDown(t *testing.T) {
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer registryServer.Close()

	uploader := newTestS3Uploader(s3Server, "")
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{s3: uploader, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/sonatype_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Healthy)
	assert.True(t, body.S3.Reachable)
	assert.False(t, body.Registry.Reachable)
	assert.NotEmpty(t, body.Registry.Error)
}

func TestHealthHandler_BothDown(t *testing.T) {
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s3Server.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer registryServer.Close()

	uploader := newTestS3Uploader(s3Server, "")
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{s3: uploader, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/sonatype_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Healthy)
	assert.False(t, body.S3.Reachable)
	assert.NotEmpty(t, body.S3.Error)
	assert.False(t, body.Registry.Reachable)
	assert.NotEmpty(t, body.Registry.Error)
}
