// Package cache provides application and HTTP response cache.
package cache

import (
	"context"
	"errors"

	"github.com/RedHatInsights/rbac-client-go"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

var NotFound = errors.New("not found in cache")

type RbacCache interface {
	GetAccessList(ctx context.Context) (rbac.AccessList, error)
	SetAccessList(ctx context.Context, accessList rbac.AccessList) error
}

func Initialize() RbacCache {
	if config.Get().Clients.Redis.Host != "" {
		return NewRedisCache()
	} else {
		log.Logger.Warn().Msg("No application cache in use")
		return NewNoOpCache()
	}
}
