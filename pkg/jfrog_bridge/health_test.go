package jfrog_bridge

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
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer jfrogServer.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer registryServer.Close()

	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{jfrog: jfrog, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/jfrog_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Healthy)
	assert.True(t, body.JFrog.Reachable)
	assert.True(t, body.Registry.Reachable)
}

func TestHealthHandler_JFrogDown(t *testing.T) {
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jfrogServer.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer registryServer.Close()

	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{jfrog: jfrog, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/jfrog_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Healthy)
	assert.False(t, body.JFrog.Reachable)
	assert.NotEmpty(t, body.JFrog.Error)
	assert.True(t, body.Registry.Reachable)
}

func TestHealthHandler_RegistryDown(t *testing.T) {
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer jfrogServer.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer registryServer.Close()

	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{jfrog: jfrog, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/jfrog_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Healthy)
	assert.True(t, body.JFrog.Reachable)
	assert.False(t, body.Registry.Reachable)
	assert.NotEmpty(t, body.Registry.Error)
}

func TestHealthHandler_BothDown(t *testing.T) {
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jfrogServer.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer registryServer.Close()

	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}

	h := &adminHandler{jfrog: jfrog, registry: registry}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/jfrog_bridge/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.health(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body healthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Healthy)
	assert.False(t, body.JFrog.Reachable)
	assert.NotEmpty(t, body.JFrog.Error)
	assert.False(t, body.Registry.Reachable)
	assert.NotEmpty(t, body.Registry.Error)
}
