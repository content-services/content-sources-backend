package cache

import (
	"context"

	"github.com/RedHatInsights/rbac-client-go"
	"github.com/content-services/content-sources-backend/pkg/api"
)

// A noop cache doesn't actually cache anything, but provides an implementation
// of the caching interfaces
type noOpCache struct {
}

func NewNoOpCache() *noOpCache {
	return &noOpCache{}
}

// GetAccessList a NoOp version to fetch a cached AccessList
func (c *noOpCache) GetAccessList(ctx context.Context) (rbac.AccessList, error) {
	return nil, NotFound
}

// SetAccessList a NoOp version to store an AccessList
func (c *noOpCache) SetAccessList(ctx context.Context, accessList rbac.AccessList) error {
	return nil
}

// GetPulpContentPath a NoOp version to fetch a cached content path
func (c *noOpCache) GetPulpContentPath(ctx context.Context) (string, error) {
	return "", NotFound
}

// SetPulpContentPath a NoOp version to store a content path
func (c *noOpCache) SetPulpContentPath(ctx context.Context, repoConfigFile string) error {
	return nil
}

// GetSubscriptionCheck a NoOp version to fetch a cached subscription check
func (c *noOpCache) GetSubscriptionCheck(ctx context.Context) (*api.SubscriptionCheckResponse, error) {
	return nil, NotFound
}

// SetSubscriptionCheck a NoOp version to store a subscription check
func (c *noOpCache) SetSubscriptionCheck(ctx context.Context, response api.SubscriptionCheckResponse) error {
	return nil
}
