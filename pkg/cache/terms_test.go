package cache

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisserver "github.com/alicebob/miniredis/v2/server"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTermsRedis(t *testing.T, server *miniredis.Miniredis) *redisCache {
	t.Helper()
	previous := config.LoadedConfig
	t.Cleanup(func() { config.LoadedConfig = previous })
	config.LoadedConfig.Loaded = true
	host, port, err := net.SplitHostPort(server.Addr())
	require.NoError(t, err)
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)
	config.Get().Clients.Redis = config.Redis{Host: host, Port: portNumber}
	c, ok := InitializeTerms().(*redisCache)
	require.True(t, ok)
	t.Cleanup(func() { require.NoError(t, c.client.Close()) })
	return c
}

func TestTermsKeyIsolation(t *testing.T) {
	scope := TermsScope{"https://terms.test/isrequired", "lightwell", "User", "network", "1"}
	seen := map[string]bool{TermsKey(scope): true}
	for _, field := range []string{"endpoint", "site", "login", "event", "version"} {
		other := scope
		switch field {
		case "endpoint":
			other.Endpoint += "/other"
		case "site":
			other.Site += ":other"
		case "login":
			other.Login = "user"
		case "event":
			other.Event = "academic"
		case "version":
			other.Version = "2"
		}
		key := TermsKey(other)
		assert.False(t, seen[key], field)
		assert.NotContains(t, key, "User")
		seen[key] = true
	}
	assert.NotEqual(t, TermsKey(TermsScope{Site: "a:b", Login: "c"}), TermsKey(TermsScope{Site: "a", Login: "b:c"}))
}

func TestTermsRedisMarkersAndExpiration(t *testing.T) {
	server := miniredis.RunT(t)
	c := testTermsRedis(t, server)
	ctx := context.Background()
	scope := TermsScope{Login: "user", Event: "network"}
	hit, err := c.GetTermsNotRequired(ctx, scope)
	require.ErrorIs(t, err, ErrNotFound)
	assert.False(t, hit)
	for _, ttl := range []time.Duration{0, -time.Second} {
		require.NoError(t, c.SetTermsNotRequired(ctx, scope, ttl))
		assert.False(t, server.Exists(TermsKey(scope)))
	}
	require.NoError(t, c.SetTermsNotRequired(ctx, scope, time.Hour))
	value, err := server.Get(TermsKey(scope))
	require.NoError(t, err)
	assert.Equal(t, "not_required", value)
	server.FastForward(10 * time.Minute)
	hit, err = c.GetTermsNotRequired(ctx, scope)
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, 50*time.Minute, server.TTL(TermsKey(scope)))
	server.FastForward(time.Hour)
	_, err = c.GetTermsNotRequired(ctx, scope)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, server.Set(TermsKey(scope), "true"))
	hit, err = c.GetTermsNotRequired(ctx, scope)
	require.Error(t, err)
	assert.False(t, hit)
	server.Del(TermsKey(scope))
	_, err = server.Lpush(TermsKey(scope), "not_required")
	require.NoError(t, err)
	hit, err = c.GetTermsNotRequired(ctx, scope)
	require.Error(t, err)
	assert.False(t, hit)
	noop := NewNoOpCache()
	require.NoError(t, noop.SetTermsNotRequired(ctx, scope, time.Hour))
	hit, err = noop.GetTermsNotRequired(ctx, scope)
	assert.False(t, hit)
	require.ErrorIs(t, err, ErrNotFound)
}

func assertTermsBudget(t *testing.T, c *redisCache) {
	t.Helper()
	parent := context.Background()
	for _, operation := range []string{"read", "write"} {
		started := time.Now()
		var err error
		if operation == "read" {
			_, err = c.GetTermsNotRequired(parent, TermsScope{})
		} else {
			err = c.SetTermsNotRequired(parent, TermsScope{}, time.Hour)
		}
		require.Error(t, err, operation)
		assert.Less(t, time.Since(started), TermsOperationTimeout+35*time.Millisecond, operation)
		require.NoError(t, parent.Err())
	}
}

func TestTermsRedisOperationBudget(t *testing.T) {
	t.Run("refused connection", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())
		options := termsRedisOptions()
		options.Addr = addr
		c := &redisCache{client: redis.NewClient(options)}
		defer c.client.Close()
		assertTermsBudget(t, c)
	})
	t.Run("stalled authentication", func(t *testing.T) {
		options := termsRedisOptions()
		options.Password = "test-password"
		options.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Consume part of the operation budget dialing; initialization gets only the remainder.
			timer := time.NewTimer(30 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			client, server := net.Pipe()
			t.Cleanup(func() { _ = server.Close() })
			return client, nil
		}
		c := &redisCache{client: redis.NewClient(options)}
		defer c.client.Close()
		assertTermsBudget(t, c)
	})
	t.Run("exhausted pool", func(t *testing.T) {
		server := miniredis.RunT(t)
		options := termsRedisOptions()
		options.Addr = server.Addr()
		options.PoolSize = 1
		options.MaxActiveConns = 1
		c := &redisCache{client: redis.NewClient(options)}
		defer c.client.Close()
		held := c.client.Conn()
		require.NoError(t, held.Ping(context.Background()).Err())
		defer held.Close()
		assertTermsBudget(t, c)
	})
	t.Run("established connection stalls", func(t *testing.T) {
		server := miniredis.RunT(t)
		c := testTermsRedis(t, server)
		require.NoError(t, c.client.Ping(context.Background()).Err())
		// Consume commands without answering, keeping the initialized socket open.
		server.Server().SetPreHook(func(peer *redisserver.Peer, cmd string, args ...string) bool { return true })
		assertTermsBudget(t, c)
	})
}
