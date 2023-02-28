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
	Allowed(xrhid string, resource string, verb RbacVerb) (bool, error)
}

type RbacConfig struct {
	client  rbac.Client
	timeout int
}

func NewRbac(baseUrl string, timeout int) Rbac {
	if baseUrl == "" {
		return nil
	}
	if timeout < 0 {
		return nil
	}
	return &RbacConfig{
		client:  rbac.NewClient(baseUrl, application),
		timeout: timeout,
	}
}

// Allowed fetches an access list from RBAC based on RBAC_URL and returns whether or not
// the xrhid has the `content-sources:*:*` permission
// xrhid is the identity json structure coded in base64.
// resource is the content-sources resource which is being requested.
// verb is the action we are quering, in our case, read or write
func (r *RbacConfig) Allowed(xrhid string, resource string, verb RbacVerb) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.timeout)*time.Second)
	defer cancel()

	acl, err := r.client.GetAccess(ctx, xrhid, "")
	if err != nil {
		return false, err
	}

	return acl.IsAllowed(application, resource, string(verb)), nil
}
