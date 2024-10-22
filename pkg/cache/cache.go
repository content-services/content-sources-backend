// Package cache provides application and HTTP response cache.
package cache

import (
	"context"
	"errors"

	"github.com/RedHatInsights/rbac-client-go"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

var NotFound = errors.New("not found in cache")

type Cache interface {
	GetAccessList(ctx context.Context) (rbac.AccessList, error)
	SetAccessList(ctx context.Context, accessList rbac.AccessList) error

	GetPulpContentPath(ctx context.Context) (string, error)
	SetPulpContentPath(ctx context.Context, pulpContentPath string) error

	GetSubscriptionCheck(ctx context.Context) (*api.SubscriptionCheckResponse, error)
	SetSubscriptionCheck(ctx context.Context, response api.SubscriptionCheckResponse) error
}

func Initialize() Cache {
	if config.Get().Clients.Redis.Host != "" {
		return NewRedisCache()
	} else {
		log.Logger.Warn().Msg("No application cache in use")
		return NewNoOpCache()
	}
}
