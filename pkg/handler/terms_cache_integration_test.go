package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/handler/lightwell"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type termsCacheAPI struct {
	terms    string
	packages string
}

type termsCacheHarness struct {
	t          *testing.T
	redis      *miniredis.Miniredis
	upstream   *httptest.Server
	router     *echo.Echo
	apis       []termsCacheAPI
	features   []string
	featureErr error
	mutex      sync.Mutex
	bodies     map[string]string
	calls      map[string]int
}

func newTermsCacheHarness(t *testing.T, eventMap string) *termsCacheHarness {
	t.Helper()
	previousConfig := config.LoadedConfig
	t.Cleanup(func() { config.LoadedConfig = previousConfig })

	h := &termsCacheHarness{
		t: t, redis: miniredis.RunT(t),
		features: []string{"lightwell-network"},
		bodies:   map[string]string{"network": "false", "academic": "true"},
		calls:    map[string]int{},
		apis: []termsCacheAPI{
			{terms: api.FullRootPath() + "/lightwell/terms/required", packages: api.FullRootPath() + "/lightwell/packages"},
			{terms: lightwell.LightwellAPIPath + "/terms/required", packages: lightwell.LightwellAPIPath + "/packages"},
		},
	}
	h.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/svcrest/terms/presentation/isrequired", r.URL.Path)
		assert.Equal(t, "lightwell", r.URL.Query().Get("site"))
		assert.Equal(t, "cache-user", r.URL.Query().Get("login"))
		event := r.URL.Query().Get("event")
		h.mutex.Lock()
		body, ok := h.bodies[event]
		h.calls[event]++
		h.mutex.Unlock()
		if !ok {
			http.Error(w, "unexpected event", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(h.upstream.Close)
	port, err := strconv.Atoi(h.redis.Port())
	require.NoError(t, err)
	config.LoadedConfig = config.Configuration{Loaded: true}
	config.LoadedConfig.Features.LightwellTerms = config.Feature{Enabled: true}
	config.LoadedConfig.Clients.TermsService = config.TermsService{
		Server: h.upstream.URL, Site: "lightwell", EventFeatureMap: eventMap, CacheVersion: "1",
	}
	config.LoadedConfig.Clients.Redis = config.Redis{
		Host: h.redis.Host(), Port: port, Expiration: config.Expiration{Terms: time.Hour},
	}
	h.registerAPIs()
	return h
}

func (h *termsCacheHarness) registerAPIs() {
	h.t.Helper()
	h.router = echo.New()
	h.router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	fs := feature_service_client.NewMockFeatureServiceClient(h.t)
	fs.EXPECT().GetEntitledFeatures(mock.Anything, "cache-org").RunAndReturn(func(context.Context, string) ([]string, error) {
		return h.features, h.featureErr
	})
	var fsClient feature_service_client.FeatureServiceClient = fs

	// Each API registration gets its own real client, sharing only Redis state.
	for i, paths := range h.apis {
		ts, err := terms_service_client.NewTermsServiceClient()
		require.NoError(h.t, err)
		if i == 0 {
			handler.RegisterLightwellTermsRoutes(h.router.Group(api.FullRootPath()), &ts, &fsClient)
		} else {
			lightwell.RegisterLightwellTermsRoutes(h.router.Group(lightwell.LightwellAPIPath), &ts, &fsClient)
		}
		h.router.GET(paths.packages, func(c echo.Context) error {
			return c.String(http.StatusOK, "package handler executed")
		}, middleware.RequireTermsAcceptance(ts, fsClient))
	}
}

func (h *termsCacheHarness) request(path string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(identity.WithIdentity(req.Context(), identity.XRHID{
		Identity: identity.Identity{
			OrgID: "cache-org", Internal: identity.Internal{OrgID: "cache-org"},
			User: &identity.User{Username: "cache-user"},
		},
	}))
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *termsCacheHarness) assertTerms(path string, events ...string) {
	h.t.Helper()
	rec := h.request(path)
	require.Equal(h.t, http.StatusOK, rec.Code, rec.Body.String())
	var response api.TermsRequiredResponse
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(h.t, len(events) > 0, response.Required)
	assert.ElementsMatch(h.t, events, response.Events)
	if len(events) > 0 {
		assert.Equal(h.t, "lightwell", response.Site)
	} else {
		assert.Empty(h.t, response.Site)
	}
}

func (h *termsCacheHarness) assertGuard(path string, blocked bool) {
	h.t.Helper()
	rec := h.request(path)
	if blocked {
		require.Equal(h.t, http.StatusPreconditionRequired, rec.Code, rec.Body.String())
		assert.JSONEq(h.t, `{"errors":[{"status":428,"title":"Terms acceptance required","detail":"You must accept the Lightwell terms of use before accessing this resource. Visit https://console.redhat.com/lightwell to accept."}]}`, rec.Body.String())
	} else {
		require.Equal(h.t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(h.t, "package handler executed", rec.Body.String())
	}
}

func (h *termsCacheHarness) setBody(event, body string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.bodies[event] = body
}

func (h *termsCacheHarness) callCount(event string) int {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.calls[event]
}

func (h *termsCacheHarness) key(event, version string) string {
	return cache.TermsKey(cache.TermsScope{
		Endpoint: h.upstream.URL + "/svcrest/terms/presentation/isrequired", Site: "lightwell",
		Login: "cache-user", Event: event, Version: version,
	})
}

func TestTermsCacheGuardSharedAllowanceAndRenewedTerms(t *testing.T) {
	for _, invalidation := range []string{"expiration", "version rotation"} {
		t.Run(invalidation, func(t *testing.T) {
			h := newTermsCacheHarness(t, "network:lightwell-network")
			h.assertTerms(h.apis[0].terms)
			require.Equal(t, 1, h.callCount("network"))
			require.Equal(t, []string{h.key("network", "1")}, h.redis.Keys())
			marker, err := h.redis.Get(h.key("network", "1"))
			require.NoError(t, err)
			assert.Equal(t, "not_required", marker)

			h.redis.FastForward(time.Minute)
			h.setBody("network", "true")
			for _, paths := range h.apis {
				h.assertTerms(paths.terms)
				h.assertGuard(paths.packages, false)
			}
			assert.Equal(t, 1, h.callCount("network"), "both clients share the allowance until invalidation")
			assert.Equal(t, 59*time.Minute, h.redis.TTL(h.key("network", "1")), "hits must not renew expiration")

			if invalidation == "expiration" {
				h.redis.FastForward(time.Hour)
			} else {
				config.LoadedConfig.Clients.TermsService.CacheVersion = "2"
				h.registerAPIs()
			}
			for _, paths := range h.apis {
				h.assertTerms(paths.terms, "network")
				h.assertGuard(paths.packages, true)
			}
			assert.Equal(t, 5, h.callCount("network"))
			if invalidation == "expiration" {
				assert.Empty(t, h.redis.Keys())
			} else {
				assert.True(t, h.redis.Exists(h.key("network", "1")), "old namespace expires naturally")
				assert.False(t, h.redis.Exists(h.key("network", "2")), "required responses never populate the active namespace")
			}
		})
	}
}

func TestTermsCacheGuardAcceptanceImmediatelyVisible(t *testing.T) {
	h := newTermsCacheHarness(t, "network:lightwell-network")
	h.setBody("network", "true")
	for _, paths := range h.apis {
		h.assertTerms(paths.terms, "network")
		h.assertGuard(paths.packages, true)
	}
	assert.Equal(t, 4, h.callCount("network"))
	assert.Empty(t, h.redis.Keys())

	h.setBody("network", "false")
	h.assertGuard(h.apis[0].packages, false)
	for _, paths := range h.apis {
		h.assertTerms(paths.terms)
		h.assertGuard(paths.packages, false)
	}
	assert.Equal(t, 5, h.callCount("network"), "the next request observes acceptance and warms both APIs")
	assert.Equal(t, []string{h.key("network", "1")}, h.redis.Keys())
}

func TestTermsCacheGuardFailuresNeverWarm(t *testing.T) {
	for _, failure := range []string{"feature service", "malformed terms response"} {
		t.Run(failure, func(t *testing.T) {
			h := newTermsCacheHarness(t, "network:lightwell-network")
			if failure == "feature service" {
				h.featureErr = errors.New("feature service unavailable")
			} else {
				h.setBody("network", "malformed")
			}
			for _, paths := range h.apis {
				h.assertTerms(paths.terms)
				h.assertGuard(paths.packages, false)
			}
			assert.Empty(t, h.redis.Keys(), "fail-open results must never create a cached allowance")
			if failure == "feature service" {
				assert.Zero(t, h.callCount("network"), "feature failures bypass the terms client")
			} else {
				assert.Equal(t, 4, h.callCount("network"))
			}

			h.featureErr = nil
			h.setBody("network", "true")
			for _, paths := range h.apis {
				h.assertTerms(paths.terms, "network")
				h.assertGuard(paths.packages, true)
			}
			assert.Empty(t, h.redis.Keys())
		})
	}
}

func TestTermsCacheGuardFiltersCurrentEntitlementsAndMappings(t *testing.T) {
	h := newTermsCacheHarness(t, "network:lightwell-network,academic:lightwell-research-institutions")
	for _, paths := range h.apis {
		h.assertTerms(paths.terms)
		h.assertGuard(paths.packages, false)
	}
	assert.Equal(t, 1, h.callCount("network"))
	assert.Equal(t, 4, h.callCount("academic"))
	assert.Equal(t, []string{h.key("network", "1")}, h.redis.Keys())

	h.features = []string{"lightwell-research-institutions"}
	for _, paths := range h.apis {
		h.assertTerms(paths.terms, "academic")
		h.assertGuard(paths.packages, true)
	}
	assert.Equal(t, 1, h.callCount("network"))
	assert.Equal(t, 8, h.callCount("academic"))

	config.LoadedConfig.Clients.TermsService.EventFeatureMap = "network:lightwell-network,academic:new-academic-feature"
	for _, paths := range h.apis {
		h.assertTerms(paths.terms)
		h.assertGuard(paths.packages, false)
	}
	h.features = []string{"new-academic-feature"}
	for _, paths := range h.apis {
		h.assertTerms(paths.terms, "academic")
		h.assertGuard(paths.packages, true)
	}
	assert.Equal(t, 1, h.callCount("network"), "accepted event remains independently cached")
	assert.Equal(t, 16, h.callCount("academic"), "outstanding event remains live through filtering changes")
	assert.Equal(t, []string{h.key("network", "1")}, h.redis.Keys())
}
