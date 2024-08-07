package rbac

import (
	"fmt"
	"strings"
)

// The following constants result from the schema below
// https://github.com/RedHatInsights/rbac-config/blob/master/schemas/permissions.schema
type Verb string

const (
	RbacVerbAny       Verb = "*"
	RbacVerbRead      Verb = "read"
	RbacVerbWrite     Verb = "write"
	RbacVerbUpload    Verb = "upload"
	RbacVerbUndefined Verb = ""
	/* Unused Verbs
	RbacVerbCreate    Verb = "create"
	RbacVerbUpdate    Verb = "update"
	RbacVerbDelete    Verb = "delete"
	RbacVerbLink      Verb = "link"
	RbacVerbUnlink    Verb = "unlink"
	RbacVerbOrder     Verb = "order"
	RbacVerbExecute   Verb = "execute" */
)

type Resource string

const (
	ResourceRepositories Resource = "repositories"
	ResourceTemplates             = "templates"
	ResourceAny          Resource = "*"
	ResourceUndefined    Resource = ""
)

type rbacEntry struct {
	resource Resource
	verb     Verb
}

var ServicePermissions *PermissionsMap = NewPermissionsMap()

// PermissionsMap Map Method and Path to a RbacEntry
type PermissionsMap map[string]map[string]rbacEntry

func NewPermissionsMap() *PermissionsMap {
	return &PermissionsMap{}
}

func (pm *PermissionsMap) Permission(method string, path string) (res Resource, verb Verb, err error) {
	path = strings.Trim(path, "/")
	if paths, ok := (*pm)[method]; ok {
		if permission, ok := paths[path]; ok {
			return permission.resource, permission.verb, nil
		}
	}
	return "", "", fmt.Errorf("no permission found for method=%s and path=%s", method, path)
}

func (pm *PermissionsMap) Add(method string, path string, res Resource, verb Verb) *PermissionsMap {
	// Avoid using empty strings
	if method == "" || path == "" || res == "" || verb == "" {
		return nil
	}
	// Avoid using of wildcard during setting the permissions map
	if res == "*" || verb == "*" {
		return nil
	}

	// Paths are stored without trailing or leading slashes, so trim them when storing
	path = strings.Trim(path, "/")
	if paths, ok := (*pm)[method]; ok {
		if permission, ok := paths[path]; ok {
			permission.resource = res
			permission.verb = verb
		} else {
			paths[path] = rbacEntry{
				resource: res,
				verb:     verb,
			}
		}
	} else {
		(*pm)[method] = map[string]rbacEntry{
			path: {
				resource: res,
				verb:     verb,
			},
		}
	}
	return pm
}
