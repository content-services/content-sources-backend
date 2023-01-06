package middleware

import "net/http"

// TODO Add this information from an external file and load it, so it can be updated without change code
// - Use embed to inject the file into the code.
// - Define a yaml file for this (easier to edit than a json file)
//

// See: https://redocly.github.io/redoc/?url=https://raw.githubusercontent.com/content-services/content-sources-backend/main/api/openapi.json
var ServicePermissions *PermissionsMap = NewPermissionsMap().
	Add(http.MethodGet, "popular_repositories", "repositories", "read").
	Add(http.MethodGet, "repositories", "repositories", "read").
	Add(http.MethodPost, "repositories", "repositories", "write").
	Add(http.MethodPost, "repositories/bulk_create", "repositories", "write").
	Add(http.MethodDelete, "repositories/:uuid", "repositories", "write").
	Add(http.MethodGet, "repositories/:uuid", "repositories", "read").
	Add(http.MethodPatch, "repositories/:uuid", "repositories", "write").
	Add(http.MethodPut, "repositories/:uuid", "repositories", "write").
	Add(http.MethodGet, "repositories/:uuid/rpms", "repositories", "read").
	Add(http.MethodGet, "repository_parameters", "repositories", "read").
	Add(http.MethodPost, "repository_parameters/validate", "repositories", "read").
	Add(http.MethodPost, "rpms/names", "repositories", "read").
	Add(http.MethodPost, "repository_parameters/external_gpg_key", "repositories", "read")
