package terms_service_client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisserver "github.com/alicebob/miniredis/v2/server"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cachedTestClient(t *testing.T, upstream *httptest.Server, redisServer *miniredis.Miniredis) *termsServiceImpl {
	t.Helper()
	previous := config.LoadedConfig
	t.Cleanup(func() { config.LoadedConfig = previous })
	config.LoadedConfig.Loaded = true
	host, port, err := net.SplitHostPort(redisServer.Addr())
	require.NoError(t, err)
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)
	config.Get().Clients.Redis = config.Redis{Host: host, Port: portNumber}
	client := newTestClient(t, upstream, "lightwell", []string{"network"})
	client.cache = cache.InitializeTerms()
	client.cacheTTL = time.Hour
	client.cacheVersion = "1"
	return client
}

func TestTermsCacheTransitionsAndSharing(t *testing.T) {
	var required atomic.Bool
	required.Store(true)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, strconv.FormatBool(required.Load()))
	}))
	defer upstream.Close()
	redisServer := miniredis.RunT(t)
	client := cachedTestClient(t, upstream, redisServer)
	check := func(c *termsServiceImpl, want bool, wantCalls int32) {
		t.Helper()
		got, err := c.IsTermsAcceptanceRequired(context.Background(), "User")
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, wantCalls, calls.Load())
	}
	check(client, true, 1)
	check(client, true, 2)
	assert.Empty(t, redisServer.Keys())
	required.Store(false)
	check(client, false, 3) // Acceptance is visible on the very next call.
	require.Len(t, redisServer.Keys(), 1)
	key := redisServer.Keys()[0]
	redisServer.FastForward(time.Minute)
	check(client, false, 3)
	assert.Equal(t, 59*time.Minute, redisServer.TTL(key))
	second := cachedTestClient(t, upstream, redisServer)
	check(second, false, 3)
	required.Store(true)
	check(second, false, 3) // Explicitly demonstrate the bounded staleness window.
	redisServer.FastForward(time.Hour)
	check(second, true, 4)
	required.Store(false)
	check(second, false, 5)
	required.Store(true)
	second.cacheVersion = "2"
	check(second, true, 6)
}

func TestTermsCacheScopeAndDisabled(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = io.WriteString(w, "false") }))
	defer upstream.Close()
	redisServer := miniredis.RunT(t)
	client := cachedTestClient(t, upstream, redisServer)
	for _, login := range []string{"User", "user", "service-account-account"} {
		_, err := client.IsTermsAcceptanceRequired(context.Background(), login)
		require.NoError(t, err)
	}
	assert.Equal(t, int32(3), calls.Load())
	client.site = "other"
	_, err := client.IsTermsAcceptanceRequired(context.Background(), "User")
	require.NoError(t, err)
	client.events = []string{"academic"}
	_, err = client.IsTermsAcceptanceRequired(context.Background(), "User")
	require.NoError(t, err)
	client.cacheVersion = "other"
	_, err = client.IsTermsAcceptanceRequired(context.Background(), "User")
	require.NoError(t, err)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = io.WriteString(w, "false") }))
	defer other.Close()
	client.endpointURL, err = buildEndpointURL(other.URL)
	require.NoError(t, err)
	_, err = client.IsTermsAcceptanceRequired(context.Background(), "User")
	require.NoError(t, err)
	assert.Equal(t, int32(7), calls.Load())
	for _, ttl := range []time.Duration{0, -time.Second} {
		client.cacheTTL = ttl
		for range 2 {
			_, err = client.IsTermsAcceptanceRequired(context.Background(), "User")
			require.NoError(t, err)
		}
	}
	assert.Equal(t, int32(11), calls.Load())
	assert.Len(t, redisServer.Keys(), 7)
	client.cacheTTL = time.Hour
	client.cache = cache.NewNoOpCache()
	for range 2 {
		_, err = client.IsTermsAcceptanceRequired(context.Background(), "User")
		require.NoError(t, err)
	}
	assert.Equal(t, int32(13), calls.Load())
}

func TestTermsCacheMixedEventsAndPartialError(t *testing.T) {
	counts := map[string]int{}
	var mu sync.Mutex
	fail := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		event := r.URL.Query().Get("event")
		mu.Lock()
		defer mu.Unlock()
		counts[event]++
		if event == "academic" {
			if fail {
				w.WriteHeader(500)
				return
			}
			_, _ = io.WriteString(w, "true")
		} else {
			_, _ = io.WriteString(w, "false")
		}
	}))
	defer upstream.Close()
	redisServer := miniredis.RunT(t)
	client := cachedTestClient(t, upstream, redisServer)
	client.events = []string{"network", "academic"}
	for range 2 {
		events, err := client.GetRequiredEvents(context.Background(), "User")
		require.NoError(t, err)
		assert.Equal(t, []string{"academic"}, events)
	}
	mu.Lock()
	assert.Equal(t, map[string]int{"network": 1, "academic": 2}, counts)
	fail = true
	mu.Unlock()
	events, err := client.GetRequiredEvents(context.Background(), "User")
	require.Error(t, err)
	assert.Nil(t, events)
	assert.Len(t, redisServer.Keys(), 1)
	// Reverse the event order: error propagation cannot depend on map iteration order.
	client.events = []string{"academic", "network"}
	events, err = client.GetRequiredEvents(context.Background(), "User")
	require.Error(t, err)
	assert.Nil(t, events)
	assert.Len(t, redisServer.Keys(), 1)
}

type failingTermsCache struct {
	cache.Cache
	read  func(context.Context) (bool, error)
	write func(context.Context) error
}

func (c failingTermsCache) GetTermsNotRequired(ctx context.Context, scope cache.TermsScope) (bool, error) {
	return c.read(ctx)
}
func (c failingTermsCache) SetTermsNotRequired(ctx context.Context, scope cache.TermsScope, ttl time.Duration) error {
	return c.write(ctx)
}

func TestTermsCacheFailuresPreserveAuthorityAndContext(t *testing.T) {
	var required atomic.Bool
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, strconv.FormatBool(required.Load()))
	}))
	defer upstream.Close()
	client := newTestClient(t, upstream, "lightwell", []string{"network"})
	client.cacheTTL = time.Hour
	writes := 0
	client.cache = failingTermsCache{read: func(ctx context.Context) (bool, error) { return true, errors.New("failed read") }, write: func(ctx context.Context) error { writes++; return errors.New("failed write") }}
	required.Store(true)
	got, err := client.IsTermsAcceptanceRequired(context.Background(), "User")
	require.NoError(t, err)
	assert.True(t, got)
	assert.Zero(t, writes)
	required.Store(false)
	for range 2 {
		got, err = client.IsTermsAcceptanceRequired(context.Background(), "User")
		require.NoError(t, err)
		assert.False(t, got)
	}
	assert.Equal(t, 2, writes)
	assert.Equal(t, int32(3), calls.Load())

	// A real timed-out cache read must leave the upstream context and fresh write usable.
	redisServer := miniredis.RunT(t)
	client = cachedTestClient(t, upstream, redisServer)
	redisServer.Server().SetPreHook(func(peer *redisserver.Peer, cmd string, args ...string) bool { return true })
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	got, err = client.IsTermsAcceptanceRequired(parent, "User")
	require.NoError(t, err)
	assert.False(t, got)
	require.NoError(t, parent.Err())
	assert.Less(t, time.Since(start), 2*cache.TermsOperationTimeout+70*time.Millisecond)
	assert.GreaterOrEqual(t, time.Since(start), 2*cache.TermsOperationTimeout-10*time.Millisecond)
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingReader) Close() error               { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTermsCacheRejectsNonAuthoritativeResponses(t *testing.T) {
	for _, body := range []string{"", "garbage", "true false", strings.Repeat(" ", maxTermsResponseBytes) + "false", "false" + strings.Repeat(" ", maxTermsResponseBytes)} {
		t.Run(strconv.Itoa(len(body))+body[:min(len(body), 5)], func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer upstream.Close()
			redisServer := miniredis.RunT(t)
			client := cachedTestClient(t, upstream, redisServer)
			_, err := client.IsTermsAcceptanceRequired(context.Background(), "User")
			require.Error(t, err)
			assert.Empty(t, redisServer.Keys())
		})
	}
	for _, scenario := range []string{"transport", "read", "non200", "cancelled", "invalid login"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
			defer upstream.Close()
			redisServer := miniredis.RunT(t)
			client := cachedTestClient(t, upstream, redisServer)
			ctx := context.Background()
			login := "User"
			switch scenario {
			case "transport":
				client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("transport failed") })
			case "read":
				client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Body: failingReader{}, Header: make(http.Header)}, nil
				})
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "invalid login":
				login = "bad login"
			}
			_, err := client.IsTermsAcceptanceRequired(ctx, login)
			require.Error(t, err)
			assert.Empty(t, redisServer.Keys())
		})
	}
}

func TestTermsCacheConcurrentRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "false") }))
	defer upstream.Close()
	client := cachedTestClient(t, upstream, miniredis.RunT(t))
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			got, err := client.IsTermsAcceptanceRequired(context.Background(), "User")
			assert.NoError(t, err)
			assert.False(t, got)
		})
	}
	group.Wait()
}
