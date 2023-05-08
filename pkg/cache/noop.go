package cache

import (
	"context"

	"github.com/RedHatInsights/rbac-client-go"
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
