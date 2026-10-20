package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/redis/go-redis/v9"
)

// TermsScope identifies one authoritative observation, independently of org and entitlements.
// Endpoint must be the normalized isrequired endpoint used by the terms client.
type TermsScope struct {
	Endpoint string
	Site     string
	Login    string
	Event    string
	Version  string
}

const TermsOperationTimeout = 50 * time.Millisecond
const termsNotRequiredMarker = "not_required"

// TermsKey uses a JSON array to avoid delimiter collisions and hashes login data.
func TermsKey(scope TermsScope) string {
	encoded, _ := json.Marshal([]string{scope.Endpoint, scope.Site, scope.Login, scope.Event, scope.Version})
	return fmt.Sprintf("terms-not-required:v1:%x", sha256.Sum256(encoded))
}

// termsRedisOptions leaves every other cache's connection policy unchanged.
func termsRedisOptions() *redis.Options {
	c := config.Get().Clients.Redis
	return &redis.Options{
		Addr: config.RedisUrl(), Username: c.Username, Password: c.Password, DB: c.DB,
		DialTimeout: TermsOperationTimeout, PoolTimeout: TermsOperationTimeout,
		ReadTimeout: TermsOperationTimeout, WriteTimeout: TermsOperationTimeout,
		ContextTimeoutEnabled: true,
		MaxRetries:            -1,
		// One dial attempt, with no dial retry backoff (zero selects library defaults).
		DialerRetries: 1,
	}
}

// InitializeTerms uses a separate bounded Redis client for terms observations only.
func InitializeTerms() Cache {
	if config.Get().Clients.Redis.Host == "" {
		return NewNoOpCache()
	}
	return &redisCache{client: redis.NewClient(termsRedisOptions())}
}

func (c *redisCache) GetTermsNotRequired(ctx context.Context, scope TermsScope) (bool, error) {
	operationCtx, cancel := context.WithTimeout(ctx, TermsOperationTimeout)
	defer cancel()
	value, err := c.get(operationCtx, TermsKey(scope))
	if err != nil {
		return false, err
	}
	if string(value) != termsNotRequiredMarker {
		return false, fmt.Errorf("invalid terms cache marker")
	}
	return true, nil
}

func (c *redisCache) SetTermsNotRequired(ctx context.Context, scope TermsScope, ttl time.Duration) error {
	if ttl <= 0 {
		return nil // Redis interprets zero as permanent storage; never pass it through.
	}
	operationCtx, cancel := context.WithTimeout(ctx, TermsOperationTimeout)
	defer cancel()
	return c.client.Set(operationCtx, TermsKey(scope), termsNotRequiredMarker, ttl).Err()
}

func (c *noOpCache) GetTermsNotRequired(ctx context.Context, scope TermsScope) (bool, error) {
	return false, ErrNotFound
}

func (c *noOpCache) SetTermsNotRequired(ctx context.Context, scope TermsScope, ttl time.Duration) error {
	return nil
}
