package client

/*
Usage example:

type myhandler struct {
	clientRbac client.Rbac
}

func (h *myhandler)myMiddleware(c echo.Context) error {
	xrhid := c.Request().Header("x-rh-identity")
	resource := mapToResource(c)
	allowed, err := h.clientRbac.Allowed(xrhid, resource, VerbRead)
	if err != nil {
		return c.Error(fmt.Error("internal error: %s", err.Error()))
	}
	if ! allowed {
		return NewHttpError(http.Unauthorized, fmt.Sprintf("you are not authorized to '%s' in '%s'", string(VerbRead), resource)
	}
	return next(c)
}
*/

import (
	"context"
	"time"

	"github.com/RedHatInsights/rbac-client-go"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
)

const application = "content-sources"

type RbacVerb string

// The following constants result from the schema below
// https://github.com/RedHatInsights/rbac-config/blob/master/schemas/permissions.schema
const (
	RbacVerbAny       RbacVerb = "*"
	RbacVerbRead      RbacVerb = "read"
	RbacVerbWrite     RbacVerb = "write"
	RbacVerbCreate    RbacVerb = "create"
	RbacVerbUpdate    RbacVerb = "update"
	RbacVerbDelete    RbacVerb = "delete"
	RbacVerbLink      RbacVerb = "link"
	RbacVerbUnlink    RbacVerb = "unlink"
	RbacVerbOrder     RbacVerb = "order"
	RbacVerbExecute   RbacVerb = "execute"
	RbacVerbUndefined RbacVerb = ""
)

type Rbac interface {
	Allowed(ctx context.Context, resource string, verb RbacVerb) (bool, error)
}

type RbacConfig struct {
	client  rbac.Client
	timeout time.Duration
	cache   cache.RbacCache
}

func NewRbac(baseUrl string, timeout time.Duration) Rbac {
	if baseUrl == "" {
		return nil
	}
	if timeout < 0 {
		return nil
	}

	return &RbacConfig{
		client:  rbac.NewClient(baseUrl, application),
		timeout: timeout,
		cache:   cache.Initialize(),
	}
}

// Allowed fetches an access list from RBAC based on RBAC_URL and returns if
// the xrhid has the `content-sources:*:*` permission
// xrhid is the identity json structure coded in base64.
// resource is the content-sources resource which is being requested.
// verb is the action we are quering, in our case, read or write
func (r *RbacConfig) Allowed(ctx context.Context, resource string, verb RbacVerb) (bool, error) {
	var acl rbac.AccessList
	var err error
	var cacheHit = false

	if r.cache != nil {
		acl, err = r.cache.GetAccessList(ctx)
		cacheHit = err == nil
		if err != cache.NotFound && err != nil {
			log.Logger.Err(err).Msg("cache error")
		}
	}
	if !cacheHit {
		reqCtx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()

		acl, err = r.client.GetAccess(reqCtx, identity.GetIdentityHeader(reqCtx), "")
		if err != nil {
			var emptyList rbac.AccessList
			err := r.cache.SetAccessList(ctx, emptyList)
			return false, err
		}
		err := r.cache.SetAccessList(ctx, acl)
		if err != nil {
			log.Error().Err(err).Msg("Failed to cache Access List")
		}
	}

	return acl.IsAllowed(application, resource, string(verb)), nil
}
