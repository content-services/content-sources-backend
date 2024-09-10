package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RedHatInsights/rbac-client-go"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/redis/go-redis/v9"
)

const PulpContentPathKey = "central-pulp-content-dir"

type redisCache struct {
	client *redis.Client
}

func NewRedisCache() *redisCache {
	c := config.Get()
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisUrl(),
		Username: c.Clients.Redis.Username,
		Password: c.Clients.Redis.Password,
		DB:       c.Clients.Redis.DB,
	})
	return &redisCache{
		client: client,
	}
}

// authKey constructs the cache key for AccessList caching
func authKey(ctx context.Context) string {
	identity := identity.GetIdentity(ctx)
	if identity.Identity.User == nil && identity.Identity.ServiceAccount != nil {
		return fmt.Sprintf("authen:%v,%v", identity.Identity.ServiceAccount.Username, identity.Identity.OrgID)
	}
	return fmt.Sprintf("authen:%v,%v", identity.Identity.User.Username, identity.Identity.OrgID)
}

// pulpContentPathKey returns the key for PulpContentPath caching
func pulpContentPathKey() string {
	return PulpContentPathKey
}

func subscriptionCheckKey(ctx context.Context) string {
	identity := identity.GetIdentity(ctx)
	return fmt.Sprintf("subscription-check:%v", identity.Identity.OrgID)
}

// GetAccessList uses the request context to read user information, and then tries to retrieve the role AccessList from the cache
func (c *redisCache) GetAccessList(ctx context.Context) (rbac.AccessList, error) {
	accessList := rbac.AccessList{}
	buf, err := c.get(ctx, authKey(ctx))
	if err != nil {
		return accessList, fmt.Errorf("redis get error: %w", err)
	}

	err = json.Unmarshal(buf, &accessList)
	if err != nil {
		return nil, fmt.Errorf("redis unmarshal error: %w", err)
	}
	return accessList, nil
}

// SetAccessList uses the request context to read user information, and loads the cache with role access list
func (c *redisCache) SetAccessList(ctx context.Context, accessList rbac.AccessList) error {
	buf, err := json.Marshal(accessList)
	if err != nil {
		return fmt.Errorf("unable to marshal for Redis cache: %w", err)
	}

	c.client.Set(ctx, authKey(ctx), string(buf), config.Get().Clients.Redis.Expiration.Rbac)
	return nil
}

func (c *redisCache) GetPulpContentPath(ctx context.Context) (string, error) {
	var contentPath string
	key := pulpContentPathKey()
	buf, err := c.get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("redis get error: %w", err)
	}

	err = json.Unmarshal(buf, &contentPath)
	if err != nil {
		return "", fmt.Errorf("redis unmarshal error: %w", err)
	}
	return contentPath, nil
}

func (c *redisCache) SetPulpContentPath(ctx context.Context, contentPath string) error {
	buf, err := json.Marshal(contentPath)
	if err != nil {
		return fmt.Errorf("unable to marshal for Redis cache: %w", err)
	}

	c.client.Set(ctx, pulpContentPathKey(), string(buf), config.Get().Clients.Redis.Expiration.PulpContentPath)
	return nil
}

func (c *redisCache) GetSubscriptionCheck(ctx context.Context) (*api.SubscriptionCheckResponse, error) {
	key := subscriptionCheckKey(ctx)
	buf, err := c.get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}

	var check api.SubscriptionCheckResponse
	err = json.Unmarshal(buf, &check)
	if err != nil {
		return nil, fmt.Errorf("redis unmarshal error: %w", err)
	}
	return &check, nil
}

func (c *redisCache) SetSubscriptionCheck(ctx context.Context, response api.SubscriptionCheckResponse) error {
	buf, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("unable to marshal for Redis cache: %w", err)
	}

	key := subscriptionCheckKey(ctx)
	c.client.Set(ctx, key, string(buf), config.Get().Clients.Redis.Expiration.SubscriptionCheck)
	return nil
}

func (c *redisCache) get(ctx context.Context, key string) ([]byte, error) {
	cmd := c.client.Get(ctx, key)
	if errors.Is(cmd.Err(), redis.Nil) {
		return nil, NotFound
	} else if cmd.Err() != nil {
		return nil, fmt.Errorf("redis error: %w", cmd.Err())
	}

	buf, err := cmd.Bytes()
	if err != nil {
		return nil, fmt.Errorf("redis bytes conversion error: %w", err)
	}
	return buf, err
}
